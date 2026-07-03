# Deploying SmellyFeet publicly via Cloudflare Tunnel

Target host: 192.168.1.135 (LAN). No router port forwarding is needed; the only
inbound path is Cloudflare -> tunnel -> `smellyfeet` container.

The tunnel connector runs as a docker-compose sidecar — do **not** install
cloudflared on the host with apt/`cloudflared service install`; the compose
stack replaces that (running both would attach two connectors to the tunnel).

## One-time Cloudflare setup (dashboard)

1. **Create the tunnel:** Zero Trust -> Networks -> Tunnels -> *Create a tunnel*
   -> Cloudflared -> name it (e.g. `smellyfeet`) -> copy the **token** (long
   string starting `eyJ`).
2. **Public hostname:** in the tunnel config, add a public hostname:
   - Subdomain/domain: `smellyfeet.<yourdomain>` (any name you like)
   - Service: `HTTP` -> `smellyfeet:3000`
   (cloudflared and the app share the compose network, so the container name
   resolves. If you instead run cloudflared directly on the host, use
   `localhost:3000` here.)
3. **Edge-cache the HTML:** Caching -> Cache Rules -> *Create rule*:
   - Name: `smellyfeet html`
   - When: Hostname equals `smellyfeet.<yourdomain>`
   - Then: **Eligible for cache**, Edge TTL: "Use cache-control header if present".
   The app sends `s-maxage` per route and `no-store` on errors, so the edge does
   the right thing.

## On the host

    git clone https://github.com/PureCypher/SmellyFeet.git && cd SmellyFeet   # or git pull
    cp deploy/.env.example deploy/.env
    # edit deploy/.env: paste TUNNEL_TOKEN, adjust API_BASE_URL if the broker
    # API is not on the host at :8080
    docker compose --project-directory deploy up -d --build

Check: `docker compose --project-directory deploy logs -f cloudflared` should
show "Registered tunnel connection". Then open `https://smellyfeet.<yourdomain>`.

## Updating

    git pull
    docker compose --project-directory deploy up -d --build

## Notes

- LAN access continues to work at http://192.168.1.135:3000.
- If the Information-Broker API runs in its own compose stack, replace the
  `API_BASE_URL` host with that stack's published address, or attach both
  stacks to a shared external network.
- Never commit `deploy/.env` (the repo `.gitignore` ignores `.env`); the tunnel
  token is a secret — rotate it in the dashboard if it leaks.
