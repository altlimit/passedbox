# 🔐 PassedBox

**Your Digital Fortress. Fully Encrypted.**

PassedBox is a local-first, zero-knowledge file vault. All encryption and decryption happens on your device — your files and keys never leave your machine. With an optional **Dead Man's Switch**, an encrypted key share can be held on a server and automatically released when you're no longer around.

> **The Vault Challenge:** We've published a vault containing the keys to a Monero wallet holding 1 XMR. If you can break it, you keep it. [Take the challenge →](https://passedbox.com/challenge)

---

## Project Structure

```
passedbox/
├── desktop/         # Desktop application (Windows, macOS, Linux)
├── server/          # Dead Man's Switch server
├── website/         # Marketing website (passedbox.com)
└── .github/         # CI/CD workflows
```

---

### 🖥️ `desktop/`

The core PassedBox desktop application built with **[Wails 3](https://wails.io)** (Go backend + Vue 3 frontend). This is what users download and run locally.

**Key features:**
- Create and manage encrypted vaults (`.pbx` files)
- AES-256-GCM encryption with Argon2id key derivation
- Shamir's Secret Sharing for vault recovery
- Optional hardware-locked passwords (tied to drive serial number)
- Dead Man's Switch integration with the server component
- Fully offline — no account or internet required

**Tech stack:** Go · Vue 3 · TypeScript · Wails 3

```bash
# Development
cd desktop
wails3 dev -config ./build/config.yml

# Production build
wails3 build
```

---

### 🌐 `server/`

The **Dead Man's Switch (DMS) server** — an optional companion that holds an encrypted key share and releases it based on configurable triggers. Your files never touch this server; only a single encrypted Shamir key share is stored.

```
server/
├── backend/       # Go API server (deployable to App Engine or Docker)
├── frontend/      # Vue 3 SPA (check-in UI, admin dashboard)
├── Dockerfile     # Multi-stage build (Node + Go → single binary)
└── docker-compose.yml
```

**Key features:**
- Vault registration with encrypted share upload
- Two independent release triggers:
  - **Credit Expiration** — share releases when prepaid credits run out
  - **Keep-Alive Check-In** — share releases when the user stops checking in
- Web push notifications and calendar feed reminders
- Admin dashboard for vault management
- Rate limiting and VAPID push support

**Tech stack:** Go · Vue 3 · TypeScript · SQLite (dsorm)

```bash
# Development
cd server/backend && air          # Go backend with hot reload
cd server/frontend && npm run dev # Vue frontend dev server

# Docker
cd server && docker compose up --build
```

---

### 🌍 `website/`

The public marketing website at [passedbox.com](https://passedbox.com), built with a lightweight static site generator ([sitegen](https://github.com/altlimit/sitegen)).

```
website/
├── site/src/       # Source HTML pages, blog posts, CSS
└── public/         # Generated static output
```

**Pages:** Home, Pricing, The Vault Challenge, Blog

```bash
cd website
sitegen -serve  # Local development with live reload
```

---

### ⚙️ `.github/`

CI/CD workflows for automated builds and deployments.

| Workflow | Trigger | What it does |
|---|---|---|
| `desktop.yml` | Push to `desktop/**` | Builds Windows/macOS/Linux apps, signs with Sigstore cosign, creates GitHub Release |
| `server.yml` | Push to `server/**` | Builds frontend, deploys to Google App Engine |
| `docker.yml` | Push to `server/**` | Builds and pushes Docker image |

---

## How It Works

1. **Create a vault** — The desktop app creates an encrypted `.pbx` file on your device
2. **Add files** — Files are encrypted with AES-256-GCM before being stored in the vault
3. **Set up recovery** — Shamir's Secret Sharing splits your master key into 3 shares:
   - **Share 1** — Stored in the vault file (requires password to decrypt)
   - **Share 2** — Given to you as a recovery key
   - **Share 3** — Optionally uploaded to a DMS server (encrypted)
4. **Dead Man's Switch** — If enabled, Share 3 is released when your credits expire or you stop checking in. Anyone with the vault file can then recover it using Share 1 + Share 3, no password needed.

> **Privacy guarantee:** Your files never leave your device. The server only ever sees an encrypted key share — it cannot decrypt your vault.

---

## License

Non-Commercial Software License — free for personal use. See [LICENSE](LICENSE) for details.
