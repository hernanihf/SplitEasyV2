# Pruebas manuales end-to-end (API)

Este documento registra las pruebas manuales que se corrieron contra la API real
(con PostgreSQL en Docker y la API de Anthropic real para el escaneo de tickets),
con el nombre de cada caso, el `curl` utilizado y el resultado esperado.

> Estas pruebas ejercitan los endpoints HTTP reales (chi + GORM + middleware JWT)
> contra una base PostgreSQL real. El flujo de login con Google OAuth se omite
> minteando un JWT a mano con el `JWT_SECRET` del `.env`, ya que ese flujo requiere
> un navegador y credenciales reales de Google.

---

## 0. Preparación del entorno

### 0.1 Levantar PostgreSQL (Docker)

```bash
docker compose up -d db

# Esperar a que acepte conexiones
until docker exec spliteasy_db pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
```

### 0.2 Levantar la API localmente

```bash
go run cmd/api/main.go
# La API queda en http://localhost:8080
```

Verificación rápida:

```bash
curl -s http://localhost:8080/ping
# {"message":"pong","status":"ok"}
```

### 0.3 Sembrar usuarios de prueba

No hay endpoint para agregar miembros a un grupo, así que se insertan dos usuarios
directamente (Alice = id 1, Bob = id 2):

```bash
docker exec spliteasy_db psql -U postgres -d spliteasy -c "
INSERT INTO users (name, email, created_at, updated_at) VALUES
 ('Alice', 'alice@test.com', now(), now()),
 ('Bob',   'bob@test.com',   now(), now())
ON CONFLICT (email) DO NOTHING;"
```

### 0.4 Mintear un JWT para Alice (user_id = 1)

El middleware sólo valida una firma HS256 con `JWT_SECRET` y lee el claim `user_id`.

```bash
export JWT_SECRET=$(grep '^JWT_SECRET=' .env | cut -d= -f2-)
python3 - << 'EOF' > /tmp/jwt.txt
import os, hmac, hashlib, base64, json, time
def b64(b): return base64.urlsafe_b64encode(b).rstrip(b'=')
secret = os.environ['JWT_SECRET'].encode()
header  = b64(json.dumps({"alg":"HS256","typ":"JWT"}, separators=(',',':')).encode())
payload = b64(json.dumps({"user_id":1,"email":"alice@test.com","exp":int(time.time())+3600}, separators=(',',':')).encode())
sig = b64(hmac.new(secret, header+b'.'+payload, hashlib.sha256).digest())
print((header+b'.'+payload+b'.'+sig).decode())
EOF

export TOKEN=$(cat /tmp/jwt.txt)
```

Todos los `curl` siguientes usan `Authorization: Bearer $TOKEN`.

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

**Esperado** (HTTP 201): grupo con `"created_by":1` y `members` conteniendo a Alice.

```json
{"id":1,"name":"Asado del finde","created_by":1,"members":[{"id":1,"name":"Alice",...}]}
```

### Caso 1.2 — Agregar a Bob al grupo (setup, vía SQL)

No hay endpoint de "add member"; se inserta la relación many2many directamente.

```bash
docker exec spliteasy_db psql -U postgres -d spliteasy -c \
  "INSERT INTO group_users (group_id, user_id) VALUES (1,2) ON CONFLICT DO NOTHING;"
```

### Caso 1.3 — Listar los grupos del usuario autenticado

```bash
curl -s http://localhost:8080/api/v1/groups -H "Authorization: Bearer $TOKEN"
```

**Esperado** (HTTP 200): array con el grupo "Asado del finde".

---

## 2. Gastos y división (split)

### Caso 2.1 — Gasto con split EQUAL (partes iguales)

Alice paga $10000, se divide en partes iguales entre los 2 miembros.

```bash
curl -s -X POST http://localhost:8080/api/v1/expenses \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"group_id":1,"paid_by_id":1,"description":"Carne","amount":10000,"split_method":"equal"}'
```

**Esperado** (HTTP 201): gasto con dos `splits` de `5000` cada uno (Alice y Bob).

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

---

## 3. Liquidar cuentas (settlements)

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

---

## 4. Escaneo de ticket con IA (Claude vision)

Requiere `ANTHROPIC_API_KEY` configurada y créditos en la cuenta. Modelo por defecto:
`claude-haiku-4-5` (configurable con `ANTHROPIC_MODEL`).

### Caso 4.1 — Escanear una foto de ticket → JSON estructurado

```bash
curl -s -X POST http://localhost:8080/api/v1/receipts/scan \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@/ruta/al/ticket.png;type=image/png"
```

**Esperado** (HTTP 200): JSON con comercio, fecha (ISO), total e ítems. Ejemplo real
obtenido con una imagen de ticket de prueba:

```json
{
  "merchant_name": "SUPERMERCADO LA ESQUINA",
  "date": "2026-06-22",
  "total_amount": 9670.5,
  "items": [
    {"description": "Leche entera 1L", "price": 1250},
    {"description": "Pan lactal", "price": 980.5},
    {"description": "Cafe molido 500g", "price": 3400},
    {"description": "Yerba mate 1kg", "price": 2150},
    {"description": "Galletitas x3", "price": 1890}
  ]
}
```

### Caso 4.2 — Sin API key configurada (error esperado)

Si `ANTHROPIC_API_KEY` está vacía, el endpoint responde con un error claro en vez de
fallar silenciosamente:

**Esperado** (HTTP 400): `receipt scanning is not configured (missing ANTHROPIC_API_KEY)`.

---

## 5. Limpieza

```bash
pkill -f "go run cmd/api/main.go"   # detener la API
docker compose down                 # detener PostgreSQL
```

---

## Resumen de cobertura

| # | Caso | Endpoint | Resultado |
|---|------|----------|-----------|
| 1.1 | Crear grupo (creador del JWT) | `POST /groups` | ✅ verificado |
| 1.3 | Listar grupos del usuario | `GET /groups` | ✅ verificado |
| 2.1 | Gasto split equal | `POST /expenses` | ✅ verificado |
| 2.2 | Balance tras equal | `GET /groups/{id}/balances` | ✅ verificado |
| 2.3 | Gasto split porcentaje | `POST /expenses` | ✅ verificado |
| 2.4 | Validación porcentajes ≠ 100 | `POST /expenses` | ✅ verificado (400) |
| 3.1 | Liquidar deuda (settle) | `POST /groups/{id}/settlements` | ✅ verificado |
| 3.2 | Balance neto tras settle | `GET /groups/{id}/balances` | ✅ verificado |
| 4.1 | Escanear ticket con IA | `POST /receipts/scan` | ✅ verificado (foto real) |

Tests automatizados de la lógica de negocio (split, balances, settle, parseo de
respuesta de IA): `internal/service/*_test.go` — `go test ./internal/service/...`.
