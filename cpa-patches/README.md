# CPA patches

`cpa/` is a git submodule pointing at upstream CLIProxyAPI, so SurplusToken's
changes to it live here as patches and must be applied before building a custom
CPA image.

## pin-by-header.patch

Lets an internal caller pin a relay request to a specific upstream OAuth account
by sending the `X-Pinned-Auth-Id: <auth.ID>` header. New API's per-account "pool"
channels set this header so each contributed account is individually addressable
(required by the contributed-account reservation pool feature).

Security: this header must only be settable inside the trusted network (New API's
per-channel HeaderOverride). Strip `X-Pinned-Auth-Id` at any public ingress to CPA.

### Apply + build

```bash
cd cpa
git apply ../cpa-patches/pin-by-header.patch
docker build -t surplustoken-cpa:latest .
```

Then point the `cpa` service in `deploy/docker-compose.yml` at `surplustoken-cpa:latest`
with `pull_policy: never`.
