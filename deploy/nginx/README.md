# deploy/nginx (optional)

Only needed if TLS is wanted in front of `hifid` for full PWA installation (docs/06 §4.4)
and the built-in TLS of `hifid` is not used. Planned content: `hifi.conf` reverse proxy with
WebSocket upgrade for `/api/v1/ws`, `client_max_body_size 0` and `proxy_request_buffering off`
for tus uploads, and a note on the own-CA certificate (mkcert) for `hifi.local`.
