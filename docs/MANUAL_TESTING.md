# Pruebas manuales end-to-end (API)

Este documento registra las pruebas manuales que se corrieron contra la API real
(con PostgreSQL en Docker, la API de Anthropic real para el escaneo de tickets, y
opcionalmente Supabase Storage / VAPID para imágenes de tickets y notificaciones
push), con el nombre de cada caso, el `curl` utilizado y el resultado esperado.

> Estas pruebas ejercitan los endpoints HTTP reales (chi + GORM + middleware JWT)
> contra una base PostgreSQL real. El flujo de login con Google OAuth se omite
> minteando un JWT a mano con el `JWT_SECRET` del `.env`, ya que ese flujo requiere
> un navegador y credenciales reales de Google. La entrega real de un push (mostrar
> la notificación en el navegador) tampoco es testeable por `curl` — esta guía
> verifica que el backend *intentó* mandarla (logs) y manejó la respuesta
> correctamente, no la UI del navegador (eso se prueba en preview, ver
> `CLAUDE.md`/sesiones de browser preview).

---

## 0. Preparación del entorno

### 0.1 Levantar PostgreSQL (Docker)

```bash
docker compose up -d db

# Esperar a que acepte conexiones
until docker exec spliteasy_db pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
```

### 0.2 Variables de entorno opcionales (features con degradación elegante)

Ninguna de estas es obligatoria para levantar la API — si falta alguna, el server
arranca igual y loguea un `WARN` explicando qué se desactivó:

| Variable | Feature que habilita | Warning si falta |
|---|---|---|
| `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_RECEIPTS_BUCKET` | Persistir imágenes de tickets escaneados | `SUPABASE_URL/SUPABASE_SERVICE_ROLE_KEY not set, receipt images will not be persisted` |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` | Notificaciones push | `VAPID_PUBLIC_KEY/VAPID_PRIVATE_KEY not set, push notifications will not be sent` |

Para probar esas dos features hace falta configurarlas en `.env`. Generar un par de
claves VAPID nuevo (no reutilizar el de producción) con un script de una sola vez:

```bash
cat > /tmp/vapidgen/main.go <<'EOF'
package main

import (
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, _ := webpush.GenerateVAPIDKeys()
	fmt.Println("VAPID_PUBLIC_KEY=" + pub)
	fmt.Println("VAPID_PRIVATE_KEY=" + priv)
}
EOF
# armar un módulo Go temporal en /tmp/vapidgen, go mod init + go get + go run
```

Para Supabase Storage, el bucket privado `receipts` tiene que existir en el proyecto
(`insert into storage.buckets (id, name, public) values ('receipts','receipts',false)`)
y `SUPABASE_SERVICE_ROLE_KEY` es la key secreta del dashboard (Project Settings → API).

### 0.3 Levantar la API localmente

```bash
go run cmd/api/main.go
# La API queda en http://localhost:8080
```

Si además vas a probar contra el frontend en preview (no solo `curl`), la API
necesita el origin del preview en la lista de CORS:

```bash
CORS_ALLOWED_ORIGINS=http://localhost:8200 go run cmd/api/main.go
```

Verificación rápida:

```bash
curl -s http://localhost:8080/ping
# {"message":"pong","status":"ok"}
```

### 0.4 Sembrar usuarios de prueba

No hay endpoint para agregar miembros a un grupo, así que se insertan dos usuarios
directamente (Alice = id 1, Bob = id 2):

```bash
docker exec spliteasy_db psql -U postgres -d spliteasy -c "
INSERT INTO users (name, email, created_at, updated_at) VALUES
 ('Alice', 'alice@test.com', now(), now()),
 ('Bob',   'bob@test.com',   now(), now())
ON CONFLICT (email) DO NOTHING;"
```

### 0.5 Mintear JWTs para Alice y Bob

El middleware sólo valida una firma HS256 con `JWT_SECRET` y lee el claim `user_id`.
Varios de los casos más abajo (comentarios, pagos, notificaciones) necesitan que dos
usuarios distintos actúen, así que conviene tener ambos tokens a mano.

```bash
export JWT_SECRET=$(grep '^JWT_SECRET=' .env | cut -d= -f2-)

mint_token() {
  local user_id=$1 email=$2
  python3 - "$user_id" "$email" << 'EOF'
import os, hmac, hashlib, base64, json, time, sys
def b64(b): return base64.urlsafe_b64encode(b).rstrip(b'=')
secret = os.environ['JWT_SECRET'].encode()
header  = b64(json.dumps({"alg":"HS256","typ":"JWT"}, separators=(',',':')).encode())
payload = b64(json.dumps({"user_id":int(sys.argv[1]),"email":sys.argv[2],"exp":int(time.time())+3600*24}, separators=(',',':')).encode())
sig = b64(hmac.new(secret, header+b'.'+payload, hashlib.sha256).digest())
print((header+b'.'+payload+b'.'+sig).decode())
EOF
}

export TOKEN=$(mint_token 1 alice@test.com)      # Alice
export TOKEN_BOB=$(mint_token 2 bob@test.com)    # Bob
```

Todos los `curl` siguientes usan `Authorization: Bearer $TOKEN` (Alice) salvo que
digan explícitamente `$TOKEN_BOB`.

---

## 1. Grupos

### Caso 1.1 — Crear grupo (el creador sale del JWT, no del body)

Valida que `created_by` se toma del usuario autenticado y que el creador queda como
primer miembro.

```bash
curl -s -X POST http://localhost:8080/api/v1/groups \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Asado del finde"}'
```

**Esperado** (HTTP 201): grupo con `"created_by":1` y `members` conteniendo a Alice,
`"currency":"USD"` (default cuando no se especifica).

```json
{"id":1,"name":"Asado del finde","created_by":1,"currency":"USD","members":[{"id":1,"name":"Alice",...}]}
```

### Caso 1.2 — Crear grupo con moneda explícita

```bash
curl -s -X POST http://localhost:8080/api/v1/groups \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Viaje a Bariloche","currency":"ARS"}'
```

**Esperado** (HTTP 201): `"currency":"ARS"`.

### Caso 1.3 — Validación: moneda desconocida (error esperado)

```bash
curl -s -X POST http://localhost:8080/api/v1/groups \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Mal","currency":"GBP"}'
```

**Esperado** (HTTP 400): `unknown currency`. Las monedas válidas hoy son USD, ARS,
BRL, MXN, EUR (`domain.CurrencyCodes`).

### Caso 1.4 — Agregar a Bob al grupo (setup, vía SQL o invite)

Sin endpoint de "add member" directo, hay dos formas:

```bash
# a) Directo por SQL (setup rápido)
docker exec spliteasy_db psql -U postgres -d spliteasy -c \
  "INSERT INTO group_users (group_id, user_id) VALUES (1,2) ON CONFLICT DO NOTHING;"

# b) Vía el flujo real de invitación (ejercita el endpoint)
TOK=$(curl -s http://localhost:8080/api/v1/groups/1/invite -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -s -X POST http://localhost:8080/api/v1/groups/join \
  -H "Authorization: Bearer $TOKEN_BOB" -H "Content-Type: application/json" \
  -d "{\"token\":\"$TOK\"}"
```

**Esperado** (b, HTTP 200): el grupo, ahora con Bob en `members`. Volver a unirse con
el mismo token es idempotente (no duplica la membresía).

### Caso 1.5 — Listar los grupos del usuario autenticado

```bash
curl -s http://localhost:8080/api/v1/groups -H "Authorization: Bearer $TOKEN"
```

**Esperado** (HTTP 200): array con los grupos creados.

### Caso 1.6 — Borrar grupo (cascada completa + limpieza de imágenes)

Setup: crear un grupo con un gasto (con ítems y splits), un pago, y comentarios en
ambos — ver Casos 2.1, 2.5 (itemizado), 3.1 y 4.1/4.2 para los `curl` de cada parte.
Con `GROUP_ID` apuntando a ese grupo:

```bash
# Confirmar que hay filas para borrar (ejemplo con expense_id/settlement_id reales)
docker exec spliteasy_db psql -U postgres -d spliteasy -c "
SELECT 'expenses', count(*) FROM expenses WHERE group_id=$GROUP_ID
UNION ALL SELECT 'settlements', count(*) FROM settlements WHERE group_id=$GROUP_ID
UNION ALL SELECT 'comments', count(*) FROM comments
  WHERE expense_id IN (SELECT id FROM expenses WHERE group_id=$GROUP_ID)
     OR settlement_id IN (SELECT id FROM settlements WHERE group_id=$GROUP_ID);"

curl -s -X DELETE http://localhost:8080/api/v1/groups/$GROUP_ID -H "Authorization: Bearer $TOKEN" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 204). Repetir la consulta SQL anterior: todos los counts en 0 —
las filas se borran de verdad (hard delete), no quedan `deleted_at` seteado. También
verificar que no quedan huérfanos en `expense_items`, `expense_item_users` ni
`expense_splits` (mismo filtro por `expense_id`), y que `group_users` y la fila de
`groups` también desaparecieron.

Si `SUPABASE_URL`/`SUPABASE_SERVICE_ROLE_KEY` están configuradas y el gasto tenía
una imagen (Caso 5.3), el archivo también se borra del bucket — para confirmarlo hay
que mirar el bucket directamente (dashboard de Supabase o `execute_sql` sobre
`storage.objects`), la propia API no expone un endpoint para listar imágenes.

### Caso 1.7 — Borrar grupo: solo el creador puede (403 esperado)

```bash
curl -s -X DELETE http://localhost:8080/api/v1/groups/$GROUP_ID -H "Authorization: Bearer $TOKEN_BOB" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 403): `only the group's creator can delete it`. El grupo sigue
existiendo.

### Caso 1.8 — Borrar un grupo inexistente (404 esperado)

```bash
curl -s -X DELETE http://localhost:8080/api/v1/groups/999999 -H "Authorization: Bearer $TOKEN" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 404): `group not found`.

---

## 2. Gastos y división (split)

### Caso 2.1 — Gasto con split EQUAL (partes iguales)

Alice paga $10000, se divide en partes iguales entre los 2 miembros.

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Carne","amount":10000,"split_method":"equal"}'
```

**Esperado** (HTTP 201): gasto con dos `splits` de `5000` cada uno (Alice y Bob), y
`"category":"other"` (default cuando se omite).

### Caso 2.2 — Balance tras el gasto equal

```bash
curl -s http://localhost:8080/api/v1/groups/1/balances -H "Authorization: Bearer $TOKEN"
```

**Esperado**: Bob le debe 5000 a Alice.

```json
[{"from_user_id":2,"to_user_id":1,"amount":5000}]
```

### Caso 2.3 — Gasto con split por PORCENTAJE

Alice paga $1000, 70% Alice / 30% Bob.

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Bebidas","amount":1000,"split_method":"percentage","splits":[{"user_id":1,"value":70},{"user_id":2,"value":30}]}'
```

**Esperado** (HTTP 201): splits de `700` (Alice) y `300` (Bob).

### Caso 2.4 — Validación: porcentajes que NO suman 100 (error esperado)

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Mal","amount":1000,"split_method":"percentage","splits":[{"user_id":1,"value":70},{"user_id":2,"value":20}]}'
```

**Esperado** (HTTP 400): `percentages must add up to 100`.

> Los métodos `fixed` (montos fijos, deben sumar el total) y `shares` (cantidades/pesos
> relativos) siguen el mismo formato de `splits[].value` — ver cobertura en
> `internal/service/expense_service_test.go`.

### Caso 2.5 — Gasto con categoría explícita, e itemizado (ítems + asignación por persona)

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Cena","amount":2000,"category":"food","split_method":"fixed",
       "splits":[{"user_id":1,"value":1000},{"user_id":2,"value":1000}],
       "items":[{"description":"Hamburguesa","amount":1000,"user_ids":[1]},{"description":"Ensalada","amount":1000,"user_ids":[2]}]}'
```

**Esperado** (HTTP 201): `"category":"food"`, y `items` con los dos ítems y sus
`users` asignados.

### Caso 2.6 — Validación: categoría desconocida (error esperado)

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Mal","amount":100,"category":"yates"}'
```

**Esperado** (HTTP 400): `unknown category`. Las categorías válidas están en
`domain.ExpenseCategorySlugs` (food, groceries, coffee, drinks, transport, fuel,
travel, accommodation, housing, utilities, internet, entertainment, sports,
shopping, health, education, gifts, pets, household, other).

### Caso 2.7 — Editar un gasto (PUT) — ⚠️ omitir `category` la resetea a "other"

A diferencia de `receipt_image_path` (ver Caso 5.3, que preserva el valor existente
si se omite), `category` es un `string` común en el request — si el cliente no lo
manda, se decodifica como `""` y `normalizeCategory` la reemplaza por el default.
El frontend siempre manda la categoría actual explícitamente para evitar esto; para
probarlo a mano:

```bash
# Sin "category" en el body → se resetea a "other"
curl -s -X PUT http://localhost:8080/api/v1/expenses/1 \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"paid_by_id":1,"description":"Carne","amount":10000,"split_method":"equal"}'
```

**Esperado** (HTTP 200): `"category":"other"`, aunque el gasto tuviera otra
categoría antes.

### Caso 2.8 — Borrar un gasto: solo el pagador o un participante del split (403 esperado)

Setup: un gasto donde ni paga ni participa un tercer usuario (o reutilizar el gasto
del Caso 2.1 con un `TOKEN` de alguien fuera del split).

```bash
curl -s -X DELETE http://localhost:8080/api/v1/expenses/1 -H "Authorization: Bearer $TOKEN_BOB" -w "\n%{http_code}\n"
# Si Bob SÍ es parte del split (Caso 2.1, split equal), esto debería dar 204
```

**Esperado**: `204` si el caller es el pagador o está en `splits`; `403` con
`you must be the payer or one of the split participants` si no.

---

## 3. Pagos (settlements)

### Caso 3.1 — Registrar un pago parcial (settle)

Bob le paga $2000 a Alice.

```bash
curl -s -X POST http://localhost:8080/api/v1/groups/1/settlements \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"from_user_id":2,"to_user_id":1,"amount":2000}'
```

**Esperado** (HTTP 201): objeto `settlement` con `amount:2000`.

### Caso 3.2 — Balance tras el settle

```bash
curl -s http://localhost:8080/api/v1/groups/1/balances -H "Authorization: Bearer $TOKEN"
```

**Esperado**: la deuda de Bob baja de 5000 a 3000 (el balance se recalcula neteando
los settlements registrados).

```json
[{"from_user_id":2,"to_user_id":1,"amount":3000}]
```

### Caso 3.3 — Borrar un pago: solo una de las dos partes (403 esperado)

```bash
docker exec spliteasy_db psql -U postgres -d spliteasy -c \
  "INSERT INTO users (name, email, created_at, updated_at) VALUES ('Carol','carol@test.com',now(),now());"
export TOKEN_CAROL=$(mint_token 3 carol@test.com)

curl -s -X DELETE http://localhost:8080/api/v1/settlements/1 -H "Authorization: Bearer $TOKEN_CAROL" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 403): `you must be a party to the settlement`.

```bash
curl -s -X DELETE http://localhost:8080/api/v1/settlements/1 -H "Authorization: Bearer $TOKEN_BOB" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 204): Bob es una de las partes (`from_user_id`), puede borrarlo.

---

## 4. Comentarios

Se comenta un gasto o un pago (exactamente uno de los dos, nunca ambos).

### Caso 4.1 — Comentar un gasto

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses/1/comments \
  -H "Authorization: Bearer $TOKEN_BOB" -H "Content-Type: application/json" \
  -d '{"body":"¿Dividimos distinto esto?"}'
```

**Esperado** (HTTP 201): comentario con `user` embebido (nombre/avatar de Bob).

### Caso 4.2 — Comentar un pago

```bash
curl -s -X POST http://localhost:8080/api/v1/settlements/1/comments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"body":"Gracias!"}'
```

**Esperado** (HTTP 201).

### Caso 4.3 — Listar comentarios de un gasto

```bash
curl -s http://localhost:8080/api/v1/expenses/1/comments -H "Authorization: Bearer $TOKEN"
```

**Esperado** (HTTP 200): array ordenado por fecha ascendente (más viejo primero).

### Caso 4.4 — Borrar comentario: solo el autor (403 esperado)

```bash
# Alice intenta borrar el comentario de Bob (Caso 4.1) → 403
curl -s -X DELETE http://localhost:8080/api/v1/comments/1 -H "Authorization: Bearer $TOKEN" -w "\n%{http_code}\n"
```

**Esperado** (HTTP 403): `you can only delete your own comment`
(`service.ErrNotCommentAuthor`). Con `$TOKEN_BOB` (el autor), da 204.

---

## 5. Escaneo de ticket con IA + persistencia de imagen

Requiere `ANTHROPIC_API_KEY` configurada y créditos en la cuenta. Modelo por defecto:
`claude-haiku-4-5` (configurable con `ANTHROPIC_MODEL`).

### Caso 5.1 — Escanear una foto de ticket → JSON estructurado

```bash
curl -s -X POST http://localhost:8080/api/v1/receipts/scan \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@/ruta/al/ticket.png;type=image/png"
```

**Esperado** (HTTP 200): JSON con comercio, fecha (ISO), total, categoría sugerida e
ítems. Ejemplo real obtenido con una imagen de ticket de prueba:

```json
{
  "merchant_name": "SUPERMERCADO LA ESQUINA",
  "date": "2026-06-22",
  "total_amount": 9670.5,
  "category": "groceries",
  "items": [
    {"description": "Leche entera 1L", "price": 1250},
    {"description": "Pan lactal", "price": 980.5}
  ]
}
```

### Caso 5.2 — Sin API key configurada (error esperado)

Si `ANTHROPIC_API_KEY` está vacía, el endpoint responde con un error claro en vez de
fallar silenciosamente:

**Esperado** (HTTP 400): `receipt scanning is not configured (missing ANTHROPIC_API_KEY)`.

### Caso 5.3 — Imagen persistida en Supabase Storage (con `SUPABASE_*` configuradas)

No hace falta una foto real para probar el guardado — cualquier imagen decodificable
sirve (el contenido no importa para este caso, solo que la subida funcione). Para
generar una JPEG mínima sin depender de herramientas externas:

```bash
cat > /tmp/makeimg.go <<'EOF'
package main

import ("image";"image/color";"image/jpeg";"os")

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	f, _ := os.Create(os.Args[1])
	defer f.Close()
	jpeg.Encode(f, img, nil)
}
EOF
go run /tmp/makeimg.go /tmp/receipt.jpg

curl -s -X POST http://localhost:8080/api/v1/receipts/scan \
  -H "Authorization: Bearer $TOKEN" -F "image=@/tmp/receipt.jpg;type=image/jpeg"
```

**Esperado** (HTTP 200): la respuesta incluye `"receipt_image_path"` (un string tipo
`abc123....jpg`) — ausente si `SUPABASE_URL`/`SUPABASE_SERVICE_ROLE_KEY` no están
configuradas (la subida es best-effort, un fallo ahí nunca rompe el scan).

Crear el gasto con ese path y confirmar que `GetExpense` devuelve una URL firmada:

```bash
PATH_VAL="<receipt_image_path de la respuesta anterior>"
EXPENSE=$(curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"group_id\":1,\"paid_by_id\":1,\"description\":\"Con foto\",\"amount\":500,\"split_method\":\"equal\",\"receipt_image_path\":\"$PATH_VAL\"}")
EXPENSE_ID=$(echo "$EXPENSE" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")

curl -s http://localhost:8080/api/v1/expenses/$EXPENSE_ID -H "Authorization: Bearer $TOKEN"
```

**Esperado**: `"receipt_image_url"` presente, una URL de
`.../storage/v1/object/sign/receipts/...?token=...` válida por ~10 minutos. Notar
que `GetGroupExpenses` (el listado) **no** incluye esta URL — solo el `GET` de un
gasto puntual la genera (evita firmar N URLs innecesarias en una lista).

### Caso 5.4 — Editar un gasto sin mandar `receipt_image_path` → se preserva

A diferencia de `category` (Caso 2.7), este campo es un puntero — si se omite en el
`PUT`, el valor existente en la base **no se toca**.

```bash
curl -s -X PUT http://localhost:8080/api/v1/expenses/$EXPENSE_ID \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"paid_by_id":1,"description":"Con foto (editado)","amount":600,"split_method":"equal"}'

curl -s http://localhost:8080/api/v1/expenses/$EXPENSE_ID -H "Authorization: Bearer $TOKEN"
```

**Esperado**: `receipt_image_url` sigue presente y apuntando al mismo archivo,
aunque el `PUT` no haya mandado `receipt_image_path`.

---

## 6. Notificaciones push

Requiere `VAPID_PUBLIC_KEY`/`VAPID_PRIVATE_KEY`/`VAPID_SUBJECT` configuradas (ver
§0.2). El envío real (que el navegador *muestre* la notificación) no es testeable
por `curl` — acá se verifica que el backend arma y manda el request HTTP firmado
correctamente, y maneja la respuesta.

### Caso 6.1 — Activar/desactivar la preferencia de un usuario

```bash
curl -s -X PATCH http://localhost:8080/api/v1/users/me/push-preference \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"push_enabled":false}' -w "\n%{http_code}\n"

docker exec spliteasy_db psql -U postgres -d spliteasy -c "SELECT id, push_enabled FROM users WHERE id=1;"
```

**Esperado**: `204`, y `push_enabled` en `f` en la fila de Alice. Volver a poner
`true` antes de seguir con los casos siguientes (si no, Alice queda excluida de
`NotifyGroupMembers` — ver Caso 6.5).

### Caso 6.2 — Registrar una suscripción

El `p256dh` tiene que decodificar a un punto EC válido y `auth` a 16 bytes — no
sirve cualquier string corto (`webpush-go` los valida antes de intentar mandar
nada). Para un `p256dh` válido de prueba, reutilizar cualquier clave pública VAPID
generada con el script de §0.2 (misma forma), y 16 bytes random en base64url para
`auth`:

```bash
AUTH=$(python3 -c "import secrets,base64;print(base64.urlsafe_b64encode(secrets.token_bytes(16)).rstrip(b'=').decode())")

curl -s -X POST http://localhost:8080/api/v1/push/subscribe \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"endpoint\":\"http://localhost:9099/fake-endpoint\",\"p256dh\":\"<clave-p256-válida>\",\"auth\":\"$AUTH\"}" \
  -w "\n%{http_code}\n"

docker exec spliteasy_db psql -U postgres -d spliteasy -c "SELECT id, user_id, endpoint FROM push_subscriptions;"
```

**Esperado** (HTTP 204): fila nueva en `push_subscriptions`. Repetir con el mismo
`endpoint` es un upsert (no duplica — `UNIQUE(user_id, endpoint)`).

### Caso 6.3 — Tope de 10 suscripciones por usuario

```bash
for i in $(seq 1 11); do
  curl -s -X POST http://localhost:8080/api/v1/push/subscribe \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d "{\"endpoint\":\"http://localhost:9099/fake-$i\",\"p256dh\":\"<clave-p256-válida>\",\"auth\":\"$AUTH\"}" \
    -w " -> %{http_code}\n" -o /dev/null
done
```

**Esperado**: las primeras 10 dan `204`; la 11ª da `400` con
`maximum of 10 push subscriptions per user reached`.

### Caso 6.4 — Dar de baja una suscripción

```bash
curl -s -X DELETE http://localhost:8080/api/v1/push/subscribe \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"endpoint":"http://localhost:9099/fake-endpoint"}' -w "\n%{http_code}\n"
```

**Esperado** (HTTP 204): la fila correspondiente desaparece de `push_subscriptions`.

### Caso 6.5 — Disparo real al crear un gasto (fire-and-forget, verificar por logs)

El push se manda en una goroutine después de responder — no bloquea ni afecta el
código de estado del request que lo disparó. Con Alice suscripta (Caso 6.2, endpoint
apuntando a algo inexistente) y Bob agregando un gasto en un grupo compartido:

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN_BOB" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":2,"description":"Push test","amount":100,"split_method":"equal"}' \
  -w "\n%{http_code}\n"
# Responde 201 casi inmediatamente

sleep 1
# Log esperado, después de la respuesta:
# level=ERROR msg="push send failed" error="...dial tcp...: connect: connection refused" subscription_id=...
```

**Esperado**: el `POST /expenses` responde `201` sin demora (el intento de push no
lo bloquea); el log del intento (éxito o fallo) aparece después, de forma asíncrona.
El mismo disparo ocurre en: editar/borrar un gasto, crear/borrar un pago, y comentar
un gasto o un pago — son los call sites de `NotifyGroupMembers` en
`expense_handler.go`, `balance_handler.go` y `comment_handler.go`.

### Caso 6.6 — Limpieza automática de una suscripción muerta (404/410)

Levantar un servidor HTTP local mínimo que devuelva 410 a cualquier POST, suscribir
un endpoint apuntando ahí, y disparar una notificación:

```bash
python3 - << 'EOF' &
from http.server import HTTPServer, BaseHTTPRequestHandler
class H(BaseHTTPRequestHandler):
    def do_POST(self): self.send_response(410); self.end_headers()
    def log_message(self, *a): pass
HTTPServer(('localhost', 9099), H).serve_forever()
EOF

curl -s -X POST http://localhost:8080/api/v1/push/subscribe \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"endpoint\":\"http://localhost:9099/gone\",\"p256dh\":\"<clave-p256-válida>\",\"auth\":\"$AUTH\"}"

curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN_BOB" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":2,"description":"Cleanup test","amount":100,"split_method":"equal"}' -o /dev/null

sleep 1
docker exec spliteasy_db psql -U postgres -d spliteasy -c \
  "SELECT * FROM push_subscriptions WHERE endpoint='http://localhost:9099/gone';"
# limpiar el servidor de prueba
kill %1
```

**Esperado**: la fila con `endpoint='.../gone'` ya no existe — un 410 (o 404) borra
la suscripción automáticamente, sin loguear error (es el comportamiento esperado del
estándar Web Push, no una falla).

### Caso 6.7 — El actor nunca se notifica a sí mismo, y `push_enabled=false` excluye

Con Alice y Bob suscriptos y ambos con `push_enabled=true`: si Alice agrega un
gasto, solo Bob debería recibir el intento de push (nunca Alice, el actor). Si antes
se pone `push_enabled=false` para Bob (Caso 6.1), el mismo gasto no debería generar
ningún intento de envío — confirmar con logs (ausencia de línea `push send failed`
o de una llamada HTTP saliente hacia el endpoint de Bob).

> Esta exclusión ya está cubierta por tests automatizados
> (`TestNotifyGroupMembers_ExcludesActorAndDisabledMembers` en
> `internal/service/push_service_test.go`) — repetirla a mano por `curl` es más para
> confirmar el comportamiento end-to-end que para encontrar bugs nuevos.

---

## 7. Limpieza

```bash
pkill -f "go run cmd/api/main.go"   # detener la API
docker compose down                 # detener PostgreSQL
```

---

## Resumen de cobertura

| # | Caso | Endpoint | Resultado |
|---|------|----------|-----------|
| 1.1 | Crear grupo (creador del JWT) | `POST /groups` | ✅ verificado |
| 1.2 | Crear grupo con moneda | `POST /groups` | ✅ verificado |
| 1.3 | Validación moneda desconocida | `POST /groups` | ✅ verificado (400) |
| 1.5 | Listar grupos del usuario | `GET /groups` | ✅ verificado |
| 1.6 | Borrar grupo (cascada + imágenes) | `DELETE /groups/{id}` | ✅ verificado |
| 1.7 | Borrar grupo: solo el creador | `DELETE /groups/{id}` | ✅ verificado (403) |
| 1.8 | Borrar grupo inexistente | `DELETE /groups/{id}` | ✅ verificado (404) |
| 2.1 | Gasto split equal | `POST /expenses` | ✅ verificado |
| 2.2 | Balance tras equal | `GET /groups/{id}/balances` | ✅ verificado |
| 2.3 | Gasto split porcentaje | `POST /expenses` | ✅ verificado |
| 2.4 | Validación porcentajes ≠ 100 | `POST /expenses` | ✅ verificado (400) |
| 2.5 | Gasto con categoría + ítems | `POST /expenses` | ✅ verificado |
| 2.6 | Validación categoría desconocida | `POST /expenses` | ✅ verificado (400) |
| 2.7 | Editar gasto (category resetea si se omite) | `PUT /expenses/{id}` | ✅ verificado |
| 2.8 | Borrar gasto: solo pagador/participante | `DELETE /expenses/{id}` | ✅ verificado (403/204) |
| 3.1 | Liquidar deuda (settle) | `POST /groups/{id}/settlements` | ✅ verificado |
| 3.2 | Balance neto tras settle | `GET /groups/{id}/balances` | ✅ verificado |
| 3.3 | Borrar pago: solo una de las partes | `DELETE /settlements/{id}` | ✅ verificado (403/204) |
| 4.1–4.3 | Comentar gasto/pago, listar | `POST/GET .../comments` | ✅ verificado |
| 4.4 | Borrar comentario: solo el autor | `DELETE /comments/{id}` | ✅ verificado (403) |
| 5.1 | Escanear ticket con IA | `POST /receipts/scan` | ✅ verificado (foto real) |
| 5.2 | Sin API key configurada | `POST /receipts/scan` | ✅ verificado (400) |
| 5.3 | Imagen persistida en Storage + URL firmada | `POST /receipts/scan`, `GET /expenses/{id}` | ✅ verificado |
| 5.4 | Editar gasto preserva receipt_image_path | `PUT /expenses/{id}` | ✅ verificado |
| 6.1 | Activar/desactivar preferencia push | `PATCH /users/me/push-preference` | ✅ verificado |
| 6.2 | Registrar suscripción | `POST /push/subscribe` | ✅ verificado |
| 6.3 | Tope de 10 suscripciones | `POST /push/subscribe` | ✅ verificado (400) |
| 6.4 | Dar de baja suscripción | `DELETE /push/subscribe` | ✅ verificado |
| 6.5 | Disparo fire-and-forget al crear gasto | `POST /expenses` (efecto lateral) | ✅ verificado (logs) |
| 6.6 | Limpieza automática de suscripción muerta | (efecto lateral) | ✅ verificado |
| 6.7 | Exclusión de actor / push_enabled=false | (efecto lateral) | ✅ cubierto por tests automatizados |

Tests automatizados de la lógica de negocio (split, balances, settle, categorías,
comentarios, storage, push, borrado en cascada, parseo de respuesta de IA):
`internal/service/*_test.go` y `internal/handler/*_test.go` —
`go test ./...` desde la raíz del repo.
