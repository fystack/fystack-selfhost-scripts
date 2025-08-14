# Release

## v0.1.5

Add `sender_name`, `sender_email` to `config.yaml`

```
email:
  resend_api_key: ""
  sender_name: "Fystack"
  sender_email: "noreply@fystack.io"
```

```sh
cd dev

docker compose pull migrate apex
docker compose up -d --no-deps --force-recreate migrate
docker compose up -d --no-deps --force-recreate apex
```
