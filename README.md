# ledgerly

**Append-only audit logs for modern SaaS.**

Ledgerly is a developer-first API that provides **immutable, cryptographically verifiable audit logs** — without building and maintaining the infrastructure yourself.

---

## Why Ledgerly?

Most audit logs are:

- Editable  
- Incomplete  
- Hard to verify  
- Built as an afterthought  

Ledgerly is different.

---

## What Ledgerly Guarantees

✅ Append-only storage  
✅ Cryptographic hash chaining  
✅ Independent integrity verification  
✅ Long-term retention  
✅ API-first design  

No dashboards you don’t need.  
No SDK lock-in.  
Just **proof**.

---

## Example Usage

```bash
curl -X POST https://api.ledgerly.io/v1/events \
  -H "Authorization: Bearer ll_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "actor": { "id": "user_123", "type": "user", "ip": "203.0.113.42" },
    "action": "project.delete",
    "resource": { "type": "project", "id": "proj_456" },
    "metadata": { "reason": "user request" }
  }'
````

Response:

```json
{
  "id": "evt_01HXYZ...",
  "occurred_at": "2025-01-15T18:32:11Z",
  "event_hash": "c4f8b3..."
}
```

---

## Endpoints (MVP)

- `POST /v1/events` – Record an audit event
    
- `GET /v1/events` – Query events
    
- `POST /v1/verify` – Verify integrity of events
    
- `POST /v1/exports` – Export events (signed download)
    

---

## Who Ledgerly is For

- B2B SaaS teams
    
- Fintech startups
    
- Healthtech applications
    
- Any team preparing for SOC 2, ISO, HIPAA, or compliance audits
    

---

## Ledgerly is Infrastructure for Trust

Ledgerly succeeds when it’s **boring to use**, **easy to trust**, and **hard to replace**.

It’s not a logging platform. It’s **proof you can show an auditor**.

