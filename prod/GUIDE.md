## Infrastructure deployment

## Certificates generation for TLS

🔑 Understanding SAN (Subject Alternative Name)

- SAN (Subject Alternative Name) is an extension in X.509 SSL/TLS certificates that allows a certificate to be valid for multiple hostnames, IP addresses, or domains.

- For NATS, you’ll want to generate certs with SANs so clients can connect using either localhost, private IP, or DNS.

- Example private IPs: 192.168.1.10, 10.0.0.5, 172.16.0.100

👉 More details: Understanding SAN in X.509 SSL Certificates (https://www.encryptionconsulting.com/understanding-san-in-x-509-ssl-certificates/)

## Steps to Create Certificates

### 1. Install mkcert and root CA

**mkcert** is a zero-config tool to generate trusted development TLS certificates:

- Automatically installs a local root CA into your system’s trust store.
- Issues certs valid for specified hostnames/IPs (e.g., localhost, 127.0.0.1).
- Supports macOS, Windows, Linux, browsers, Java—BSD-3-Clause licensed :contentReference[oaicite:10]{index=10}.

Follow instruction on Github to install mkcert: https://github.com/FiloSottile/mkcert

```

# Example: for localhost and a private IP
mkcert -cert-file server-cert.pem -key-file server-key.pem localhost 127.0.0.1 ::1 <PRIVATE_IP>
```

This generates:

- server-cert.pem → TLS certificate for NATS server

- server-key.pem → Private key

### 2. Generate Client Certificates (mTLS)

```
mkcert -client -cert-file client-cert.pem -key-file client-key.pem <CLIENT_HOSTNAME>
```

Example:

```

mkcert -client -cert-file client-cert.pem -key-file client-key.pem host1.example.com
```

IP/Hostnames Are Not Needed for Clients

### 3. Copy root CA into cert directory

```
cp "$(mkcert -CAROOT)/rootCA.pem" ./rootCA.pem
```

Now your ./cert folder will contain:

```
./cert/
 ├── rootCA.pem      # Root CA (needed by clients to verify server cert)
 ├── server-cert.pem # NATS server certificate
 ├── server-key.pem  # NATS server private key
 ├── client-cert.pem # (optional) Client certificate
 └── client-key.pem  # (optional) Client private key
```

### 4. Distribute Certificates

- Server-side (NATS node): Needs server-cert.pem, server-key.pem, and rootCA.pem.

Client-side (mpcium nodes, Apex backend): Needs client-cert.pem, client-key.pem, and rootCA.pem to establish mTLS with the NATS server.

Later on, we need to copy client side certs (`client-cert.pem`, `client-key.pem`, and `rootCA.pem`) to `/opt/mpcium/certs` folder of mpcium nodes: https://github.com/fystack/mpcium/tree/master/deployments/systemd#step-2-configure-permissions

## 2. Deploy NATS, MongoDB on Docker

### 1. Change NATS Password

> ⚠️ **Important:** Always back up your NATS password securely in a vault, pasword manager.

#### Generate bcrypt Password on Linux

```bash
htpasswd -bnBC 12 "" "your_password" | tr -d ':\n'
```

This outputs a bcrypt hash that you’ll use in the Docker Compose file.

#### Example Docker Compose Snippet

```
"--pass",
"$2a$11$2Er0oPbbN2JsPbSbvZi4AOyrXGkFBRib/fAj6wtV.Aw8KFJgtEwlq", // change this value

```

Reference: https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/username_password

### 2. Change MongoDB Password

Change password of MongoDB in `docker-compose.yaml`

```
MONGO_INITDB_ROOT_PASSWORD: "3!Z5c?\*r7x?;"

```

### 3. Deploy Services

Run the following command:

```

docker compose up -d

```
