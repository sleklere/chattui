# Inbox: mark as read

## Objetivo

Permitir marcar entradas del inbox como leídas, reseteando el `unread_count` a 0. El marcado puede dispararse automáticamente al abrir un room o DM desde la pantalla de inbox, o manualmente desde la misma pantalla.

## Alcance

| Incluido | Excluido |
|----------|----------|
| `POST /api/v1/inbox/read` — marca una entrada o todo el inbox | Marcar como leído desde el chat screen |
| Auto-mark al abrir room/DM desde inbox | Marcar como no leído |
| Tecla manual en inbox screen (entry individual) | Persistir `last_read_message_id` |
| Tecla para marcar todo como leído | Notificación WS al hacer mark as read |

## Requerimientos

**Endpoint**
- `POST /api/v1/inbox/read` acepta tres formas de body:
  - `{"room_id": X}` — marca la entrada de ese room como leída
  - `{"conversation_id": X}` — marca la entrada de esa conversación como leída
  - `{}` — marca todas las entradas del usuario como leídas
- Solo afecta las entradas del usuario autenticado
- Responde `204 No Content` en todos los casos

**TUI — inbox screen**
- Al presionar `enter` sobre una entry: navega al room/DM Y dispara mark as read para esa entry
- Tecla `x`: marca la entry seleccionada como leída sin navegar
- Tecla `X` (mayúscula): marca todas las entries como leídas
- Tras marcar como leído, el badge (unread count) de la entry se resetea a 0 en la vista

## Casos borde y errores

- Body con `room_id` y `conversation_id` simultáneamente: responder `400` — son mutuamente excluyentes
- Entry que no pertenece al usuario: la query filtra por `user_id` del token, no hace falta error explícito — simplemente no afecta nada
- Entry con `unread_count` ya en 0: idempotente, no es error
- Error en el server al marcar: loguear y seguir — no bloquear la navegación al room/DM

## Tests

El inbox service y los handlers tienen tests de integración existentes. Agregar casos que verifiquen:
- Mark individual por room y por conversation resetea `unread_count` a 0
- Mark all resetea todos los cursors del usuario
- No afecta cursors de otros usuarios
