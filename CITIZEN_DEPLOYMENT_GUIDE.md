# Citizen Deployment Guide for Citizen 

This guide contains all the information you need to deploy your applications to Citizen.

## Table of Contents

1. [Deployment Methods](#deployment-methods)
2. [project.toml Usage](#projecttoml-usage)
3. [Dockerfile Usage](#dockerfile-usage)
4. [Buildpack Usage](#buildpack-usage)
5. [Port Configuration](#port-configuration)
6. [Environment Variables](#environment-variables)
7. [Examples](#examples)
8. [Troubleshooting](#troubleshooting)
9. [LLM Prompt](#llm-prompt)

## Deployment Methods

Citizen supports 3 different deployment methods:

### 1. Buildpack (Automatic)
- Dokku automatically detects your project's language
- Uses Heroku buildpacks
- Easiest method

### 2. Dockerfile
- Allows you to create custom Docker images
- Full control
- Production-ready

### 3. project.toml
- Dokku-specific configuration
- Port, domain, and other settings
- Used together with Buildpack

## project.toml Usage

The `project.toml` file determines how Dokku will deploy your application.

### Basic Structure

```toml
[project]
id = "my-app"
name = "My Application"
version = "1.0.0"

[dokku]
port = 3000

[deploy]
port = 3000
health_check = "/health"

[build.env]
NODE_ENV = "production"
```

### Detailed Example

```toml
[project]
id = "nodejs-app"
name = "Node.js Application"
version = "1.0.0"

# Dokku-specific settings
[dokku]
port = 8080
domain = "myapp.example.com"

# Deploy settings
[deploy]
port = 8080
health_check = "/api/health"

# Build-time environment variables
[build.env]
NODE_ENV = "production"
NPM_CONFIG_PRODUCTION = "false"

# Metadata (alternative port definition)
[metadata.dokku]
port = 8080

[metadata.deploy]
port = 8080
```

### Port Priority Order

Citizen searches for the port in the following order:
1. `metadata.dokku.port`
2. `metadata.deploy.port`
3. `dokku.port`
4. `deploy.port`
5. `build.env.PORT`

## Dockerfile Usage

You can create custom images using Dockerfile.

### Important Dokku Requirements

When using Dockerfile with Dokku, you **MUST** include the `EXPOSE` directive to specify which port your application listens on. Without `EXPOSE`, Dokku will default to port 5000, which may not match your application's actual port.

```dockerfile
# REQUIRED: Expose the port your app listens on
EXPOSE 3000
```

### Node.js Example

```dockerfile
# Build stage
FROM node:18-alpine AS builder

WORKDIR /app

# Dependencies
COPY package*.json ./
RUN npm ci --only=production

# Copy source
COPY . .

# Production stage
FROM node:18-alpine

WORKDIR /app

# Copy from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app .

# Non-root user
RUN addgroup -g 1001 -S nodejs
RUN adduser -S nodejs -u 1001
USER nodejs

# IMPORTANT: Expose the port your app listens on
EXPOSE 3000

# Start
CMD ["node", "server.js"]
```

### Python Example

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source
COPY . .

# Non-root user
RUN useradd -m -u 1001 appuser
USER appuser

# IMPORTANT: Expose the port your app listens on
EXPOSE 8000

# Start
CMD ["gunicorn", "--bind", "0.0.0.0:8000", "app:app"]
```

### Go Example

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Production stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary
COPY --from=builder /app/main .

# IMPORTANT: Expose the port your app listens on
EXPOSE 8080

# Start
CMD ["./main"]
```

### Dockerfile Port Behavior

When using Dockerfile:
- **With EXPOSE**: Dokku will proxy the exposed port(s) to the same port numbers publicly
- **Without EXPOSE**: Dokku defaults to port 5000 and expects your app to respect the PORT environment variable
- **Multiple EXPOSE**: Each exposed port will be mapped (e.g., `EXPOSE 3000` and `EXPOSE 3001` will both be accessible)

To change the exposed port mapping after deployment:
```bash
# Add a port mapping to port 80
dokku proxy:ports-add app-name http:80:3000

# Remove the default port mapping
dokku proxy:ports-remove app-name http:3000:3000
```

## Buildpack Usage

Buildpacks are automatically detected, but you can also set them manually.

### Supported Buildpacks

1. **Node.js**
   - Automatically detected when `package.json` exists
   - Heroku Node.js buildpack

2. **Python**
   - When `requirements.txt` or `setup.py` exists
   - Heroku Python buildpack

3. **Go**
   - When `go.mod` exists
   - Heroku Go buildpack

4. **Ruby**
   - When `Gemfile` exists
   - Heroku Ruby buildpack

5. **PHP**
   - When `composer.json` exists
   - Heroku PHP buildpack

### Manual Buildpack Setting

Via API:
```bash
# Add buildpack
POST /api/v1/citizen/apps/{app_name}/buildpacks
{
  "buildpack": "https://github.com/heroku/heroku-buildpack-nodejs"
}

# List buildpacks
GET /api/v1/citizen/apps/{app_name}/buildpacks
```

## Port Configuration

### 1. With project.toml

```toml
[dokku]
port = 3000
```

### 2. Via API

```bash
POST /api/v1/citizen/apps/{app_name}/port
{
  "port": 3000
}
```

### 3. With Environment Variable

```bash
POST /api/v1/citizen/apps/{app_name}/env
{
  "key": "PORT",
  "value": "3000"
}
```

## Environment Variables

### Setting via API

```bash
# Single variable
POST /api/v1/citizen/apps/{app_name}/env
{
  "key": "DATABASE_URL",
  "value": "postgres://user:pass@host:5432/db"
}

# Multiple variables
POST /api/v1/citizen/apps/{app_name}/config
{
  "DATABASE_URL": "postgres://...",
  "REDIS_URL": "redis://...",
  "NODE_ENV": "production"
}
```

### With project.toml

```toml
[build.env]
NODE_ENV = "production"
NPM_CONFIG_PRODUCTION = "false"
```

## Examples

### Next.js Application

**project.toml:**
```toml
[project]
id = "nextjs-app"
name = "Next.js Application"
version = "1.0.0"

[dokku]
port = 3000

[build.env]
NODE_ENV = "production"
NEXT_TELEMETRY_DISABLED = "1"
```

**Dockerfile:**
```dockerfile
FROM node:18-alpine AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:18-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:18-alpine AS runner
WORKDIR /app

ENV NODE_ENV production

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

# IMPORTANT: Must expose the port
EXPOSE 3000

ENV PORT 3000

CMD ["node", "server.js"]
```

### Express.js API

**project.toml:**
```toml
[project]
id = "express-api"
name = "Express API"
version = "1.0.0"

[dokku]
port = 8080

[deploy]
health_check = "/api/health"

[build.env]
NODE_ENV = "production"
```

### Django Application

**project.toml:**
```toml
[project]
id = "django-app"
name = "Django Application"
version = "1.0.0"

[dokku]
port = 8000

[build.env]
DJANGO_SETTINGS_MODULE = "myproject.settings.production"
DISABLE_COLLECTSTATIC = "0"
```

**Procfile:**
```
web: gunicorn myproject.wsgi --bind 0.0.0.0:$PORT
release: python manage.py migrate
```

## Troubleshooting

### Port Detection Issues

1. **Problem:** Application running on wrong port
   - **Solution:** Explicitly specify port in project.toml
   - **Alternative:** Set PORT environment variable
   - **For Dockerfile:** Ensure you have EXPOSE directive

2. **Problem:** Health check failing
   - **Solution:** Set correct deploy.health_check endpoint
   - **Check:** Ensure your app returns 200 OK at the specified endpoint

### Build Issues

1. **Problem:** Buildpack not detected
   - **Solution:** Manually add buildpack
   - **Check:** Are language-specific files in project root?

2. **Problem:** Build failing
   - **Solution:** Check build logs
   - **Alternative:** Use Dockerfile

### Deployment Issues

1. **Problem:** Container not starting
   - **Solution:** Check logs: `GET /api/v1/citizen/apps/{app_name}/logs`
   - **Check:** Is port binding correct?
   - **For Dockerfile:** Is EXPOSE directive present?

2. **Problem:** Environment variables not working
   - **Solution:** Re-set via API
   - **Check:** Distinguish between build-time vs runtime variables

## LLM Prompt

Paste the following prompt into your LLM (ChatGPT, Claude, etc.) to create customized deployment configurations for Citizen :

```
I'm deploying an app to Dokku using Citizen . Citizen  is a web-based management interface for Dokku with these features:

1. GitHub integration with automatic deployment
2. project.toml, Dockerfile, or Buildpack support
3. Port configuration (project.toml's dokku.port or via API)
4. Environment variable management
5. Custom domain support
6. Build and deployment logs

My application:
- Language/Framework: [Enter here]
- Port: [Enter here]
- Special requirements: [Enter here]

Please create for me:
1. An appropriate project.toml file
2. Optimized Dockerfile if needed (with EXPOSE directive)
3. List of required environment variables
4. Post-deployment verification steps
5. Potential issues and solutions

Citizen 's port priority order:
- metadata.dokku.port
- metadata.deploy.port
- dokku.port
- deploy.port
- build.env.PORT

If using Dockerfile:
- Must include EXPOSE directive for the port
- Use multi-stage build
- Add non-root user
- Apply security best practices
- Make it production-ready

If using project.toml:
- Add all port definitions
- Specify health check endpoint
- Add build-time environment variables
```

## Additional Resources

- [Dokku Official Documentation](https://dokku.com/docs/)
- [Heroku Buildpacks](https://devcenter.heroku.com/articles/buildpacks)
- [Docker Best Practices](https://docs.docker.com/develop/dev-best-practices/)

---

This guide contains all the information needed for successful deployments via Citizen . For questions, please use GitHub Issues. 
