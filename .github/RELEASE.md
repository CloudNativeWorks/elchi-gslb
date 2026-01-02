# Release Process

This document describes how to create a new release for the Elchi GSLB CoreDNS plugin.

## Versioning

We follow [Semantic Versioning](https://semver.org/) (SemVer):

- **MAJOR** version for incompatible API changes
- **MINOR** version for new functionality in a backward-compatible manner
- **PATCH** version for backward-compatible bug fixes

Format: `vMAJOR.MINOR.PATCH` (e.g., `v1.2.3`)

## Release Types

### Patch Release (v1.0.x)
Bug fixes, security patches, documentation updates

```bash
git tag v1.0.1
git push origin v1.0.1
```

### Minor Release (v1.x.0)
New features, enhancements (backward-compatible)

```bash
git tag v1.1.0
git push origin v1.1.0
```

### Major Release (vx.0.0)
Breaking changes, major refactoring

```bash
git tag v2.0.0
git push origin v2.0.0
```

## Creating a Release

### 1. Prepare the Release

```bash
# Ensure you're on main branch
git checkout main
git pull origin main

# Run all tests locally
make test

# Run linters
go vet ./...
staticcheck ./...
golangci-lint run

# Run security scan
gosec ./...
```

### 2. Update Version Information

Update any version references in:
- `README.md` (if showing version examples)
- `CHANGELOG.md` (create if doesn't exist)

### 3. Create and Push Tag

```bash
# Determine next version (e.g., v1.0.0)
VERSION="v1.0.0"

# Create annotated tag
git tag -a $VERSION -m "Release $VERSION"

# Push tag to trigger release workflow
git push origin $VERSION
```

### 4. Monitor GitHub Actions

1. Go to https://github.com/cloudnativeworks/elchi-gslb/actions
2. Watch the "Release" workflow run
3. The workflow will:
   - Run all tests
   - Run security scans
   - Build binaries for multiple platforms
   - Create GitHub release with changelog
   - Upload release artifacts

### 5. Verify Release

1. Check the release page: https://github.com/cloudnativeworks/elchi-gslb/releases
2. Verify all artifacts are uploaded:
   - `coredns-elchi-vX.Y.Z-linux-amd64.tar.gz`
   - `coredns-elchi-vX.Y.Z-linux-arm64.tar.gz`
   - `coredns-elchi-vX.Y.Z-darwin-amd64.tar.gz`
   - `coredns-elchi-vX.Y.Z-darwin-arm64.tar.gz`
   - SHA256 checksums for each
3. Download and verify a binary:
   ```bash
   curl -LO https://github.com/cloudnativeworks/elchi-gslb/releases/download/$VERSION/coredns-elchi-$VERSION-linux-amd64.tar.gz
   sha256sum -c coredns-elchi-$VERSION-linux-amd64.tar.gz.sha256
   ```

## Rolling Back a Release

If a release has critical issues:

```bash
# Delete the tag locally
git tag -d $VERSION

# Delete the tag remotely
git push origin :refs/tags/$VERSION

# Delete the GitHub release manually through the UI
```

## Automated Changelog

The release workflow automatically generates a changelog from git commits between tags.

### Commit Message Format

Use conventional commit format for better changelogs:

- `feat: add new feature` → New feature
- `fix: resolve bug` → Bug fix
- `docs: update readme` → Documentation
- `refactor: improve code` → Code improvement
- `test: add tests` → Testing
- `chore: update deps` → Maintenance

Example:
```bash
git commit -m "feat: add support for TXT records"
git commit -m "fix: handle nil pointer in cache lookup"
git commit -m "docs: add Kubernetes deployment example"
```

## Release Checklist

- [ ] All tests passing locally
- [ ] All linters passing (golangci-lint, staticcheck, gosec)
- [ ] Version bumped appropriately (semver)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (if exists)
- [ ] Tag created with `v` prefix
- [ ] Tag pushed to GitHub
- [ ] GitHub Actions workflow completed successfully
- [ ] Release artifacts verified
- [ ] Release notes reviewed

## Troubleshooting

### Release workflow fails

Check the workflow logs in GitHub Actions:
1. Go to Actions tab
2. Click on failed workflow
3. Review error messages
4. Fix issues and create new tag

### Missing artifacts

Re-run the workflow:
1. Go to failed workflow
2. Click "Re-run all jobs"

### Wrong version number

Delete tag and recreate:
```bash
git tag -d v1.0.0
git push origin :refs/tags/v1.0.0
git tag -a v1.0.1 -m "Release v1.0.1"
git push origin v1.0.1
```

## Questions?

Open an issue: https://github.com/cloudnativeworks/elchi-gslb/issues
