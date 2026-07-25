# Reconexión automática del cliente WebSocket

## Objetivo

El cliente WS no se reconecta si pierde la conexión con el servidor. Al caerse el back, el cliente queda sin mensajes en tiempo real y hay que reiniciarlo manualmente. Implementar reconexión automática con backoff exponencial para que el cliente se recupere solo.

## Alcance

| Incluido | Excluido |
|---|---|
| Reconexión automática al detectar desconexión | Re-join de rooms por WS (no aplica: se hace por HTTP) |
| Backoff exponencial entre intentos | Persistencia de mensajes enviados durante la desconexión |
| Limpiar el error en pantalla al reconectar | Notificación visual de "reconectando..." |

## Requerimientos

- Al perder la conexión WS, el cliente reintenta conectarse automáticamente sin intervención del usuario
- Los reintentos usan backoff exponencial (ej: 1s, 2s, 4s... hasta un máximo de ~30s)
- El puntero `*ws.Client` que usa el app no cambia — la reconexión es interna al cliente WS
- Al reconectar exitosamente, se envía `ws.ConnectedMsg` al programa bubbletea para que las pantallas activas puedan limpiar el mensaje de error
- Los mensajes enviados con `Send()` durante la desconexión se descartan (no se bufferean para reenvío)

## Componentes involucrados

- `cmd/client/internal/ws/client.go` — toda la lógica de reconexión va acá
- `cmd/client/internal/ui/chat/model.go` — manejar `ws.ConnectedMsg` para limpiar `m.err`
- `cmd/client/internal/ui/dmchat/model.go` — ídem

## Casos borde

- El servidor no levanta: el cliente sigue reintentando indefinidamente con el backoff máximo
- El contexto se cancela (cierre del cliente): el supervisor debe detenerse sin reintentar
- `Send()` mientras está desconectado: descartar silenciosamente (el buffer tiene tamaño fijo, no bloquear)

## Notas de implementación

- Diseño sugerido: goroutine supervisor que llama a `runWithConn(ctx, conn)` — corre readLoop y writeLoop para una conn dada y retorna cuando alguno falla. Al retornar, el supervisor reintenta el dial con backoff
- `readLoop` y `writeLoop` reciben `conn` como parámetro en lugar de leerlo del struct (evita mutex)
- `Send()` puede usar `select` con `default` para no bloquear si el buffer está lleno
