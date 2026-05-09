# CI/CD Pipeline

## Automated Flow

Every push to the main branch triggers the following **automated** workflow:

```
┌─────────────────────────────────────────────────────┐
│              Push to Main Branch                    │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│  1. Test & Quality Checks (Parallel)               │
│     ├─ Unit Tests + Integration Tests              │
│     ├─ Golangci-lint + Staticcheck                 │
│     └─ Security Scan (gosec + SARIF)               │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│  2. Build Binary                                    │
│     └─ CoreDNS + Elchi Plugin (AMD64)              │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│  3. Release (If VERSION changed)                   │
│     ├─ Create Git Tag (v0.1.0)                     │
│     ├─ Create GitHub Release                       │
│     ├─ Generate Changelog                          │
│     └─ Upload Source Archive                       │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│  4. Docker Build & Push                             │
│     └─ AMD64 Image Build (GitHub Runner)           │
└──────────────────┬──────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────┐
│  5. Docker Manifest                                 │
│     ├─ Versioned Tag (v0.1.0)                      │
│     └─ Latest Tag Update                           │
└─────────────────────────────────────────────────────┘
```

## Usage

### 1. Normal Development (Every Push)
```bash
# Make code changes
git add .
git commit -m "feat: add new feature"
git push origin main
```

**Result**: Test + Lint + Security run, binary built. If no release is needed, only these steps execute.

### 2. Creating a Release
```bash
# Update VERSION file
echo "0.1.1" > VERSION

# Commit and push
git add VERSION
git commit -m "chore: bump version to 0.1.1"
git push origin main
```

**Result**: Full pipeline executes:
1. ✅ Tests pass
2. ✅ Build success
3. ✅ Release v0.1.1 created
4. ✅ AMD64 Docker image built
5. ✅ Pushed to Docker Hub

## Requirements

### GitHub Secrets
The following secrets must be defined in repository settings:

- `DOCKER_USERNAME`: Docker Hub username
- `DOCKER_PASSWORD`: Docker Hub access token

## Workflow Files

- **Active**: `.github/workflows/ci.yml` - Complete pipeline
- **Disabled**: `.github/workflows/disabled/` - Old separate workflows (backup)

## Features

✅ Fully automated - No user intervention required
✅ Prevents duplicate releases (version check)
✅ Release created only when VERSION changes
✅ AMD64 Docker images (linux/amd64)
✅ Integrated with GitHub Security (SARIF upload)
✅ Artifact uploads (binary downloads)

## Troubleshooting

### Release not created
- Check if VERSION file changed
- Check if release already exists for this version

### Docker push failed
- Verify `DOCKER_USERNAME` and `DOCKER_PASSWORD` secrets
- Verify repository exists on Docker Hub (`cloudnativeworks/elchi-coredns`)
