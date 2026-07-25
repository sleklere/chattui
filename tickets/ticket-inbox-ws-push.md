# Inbox: push de actualizaciones en tiempo real vía WebSocket

## Objetivo

Cuando llega un nuevo mensaje (DM o de sala), el inbox del destinatario debe actualizarse en tiempo real sin requerir un re-fetch manual. Hoy el inbox solo se carga al entrar a la pantalla correspondiente.

## Alcance

| Incluido | Excluido |
|----------|----------|
| Push WS al destinatario cuando llega un DM | Mark as read |
| Push WS a miembros de sala cuando llega un mensaje de sala | Badges / unread count en tab bar |
| TUI actualiza inbox al recibir el evento | Reconexión automática al WS |

## Requerimientos

- Cuando un usuario recibe un DM, su inbox se actualiza automáticamente si está conectado vía WS
- Cuando se envía un mensaje en una sala, todos los miembros conectados ven su inbox actualizado
- El evento WS lleva los datos completos de la entrada de inbox afectada (mismo contenido que devuelve `GET /inbox`)
- Si el usuario no está conectado al WS al momento del evento, no pasa nada — el inbox se carga normalmente al reconectar

## Componentes involucrados

- `inbox.Service` — genera y publica el evento con la entry actualizada
- Bus de eventos — transporta el evento a los suscriptores
- `ws.Hub` — recibe el evento del bus y lo pushea al cliente WS correspondiente
- TUI inbox screen — maneja el evento entrante y actualiza la vista

## Casos borde

- El sender también recibe la actualización en su inbox
- Un usuario conectado en múltiples dispositivos (mismo userID) recibe el push en la conexión activa
- Si la entry no existe aún en el inbox del destinatario (primera interacción), el evento igual debe reflejar el estado correcto

## Tests

El hub ya tiene tests unitarios. Agregar casos que verifiquen que al publicar un evento de DM o mensaje de sala en el bus, los clientes WS correspondientes reciben un mensaje de tipo `inbox_updated` con payload válido.
