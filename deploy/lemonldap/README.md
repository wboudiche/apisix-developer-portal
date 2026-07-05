# LemonLDAP::NG test OIDC provider

Baked LemonLDAP::NG configuration providing an OIDC issuer at
`http://auth.example.com` for local/dev/test use, with a relying party
configured for the **client-credentials grant** issuing **JWT access
tokens**, verifiable via **JWKS**. This is what the developer portal's
"OAuth2 for consumers" feature talks to in the full test stack.

## Image

```
coudot/lemonldap-ng:2.20.2
digest: sha256:604100fada8db7a2218e460fcff411d99a3623c022ab3d9fae1cfd43c00aea65
```

The brief suggested `coudot/lemonldap-ng:2.19` — that exact tag does not
exist on Docker Hub (only `2.19.0`/`2.19.1`/`2.19.2`). `2.20.2` was the
newest published stable tag at the time this was produced (2026-07-05) and
was used instead.

## Client credentials (Task 4 `OIDC_ISSUER` config)

| Setting | Value |
|---|---|
| Issuer | `http://auth.example.com` |
| Token endpoint | `http://auth.example.com/oauth2/token` |
| JWKS endpoint | `http://auth.example.com/oauth2/jwks` |
| Discovery | `http://auth.example.com/.well-known/openid-configuration` |
| `client_id` | `apisix-portal-app` (RP 1) / `apisix-portal-app2` (RP 2) |
| `client_secret` | `apisix-portal-secret` / `apisix-portal-secret2` |
| Grant type | `client_credentials` (send `scope=openid`, see note below) |

Two relying parties are seeded so the OAuth2 E2E test (`internal/e2e`,
`TestOAuth2TwoClients`) can bind a distinct real client id to each API and
prove cross-client rejection (a token for RP 1 gets 403 on RP 2's API).

## THE key finding: which claim carries the client id

**`client_id`** — a top-level claim, always present, always equal to the
requesting RP's client ID (`apisix-portal-app`), regardless of scope or
grant configuration.

**Set `OIDC_CLIENT_ID_CLAIM=client_id` for Task 4.**

This was confirmed by reading LemonLDAP::NG's own source
(`Lemonldap::NG::Portal::Lib::OpenIDConnect::makeJWT`, in the
`coudot/lemonldap-ng:2.20.2` image at
`/usr/share/perl5/Lemonldap/NG/Portal/Lib/OpenIDConnect.pm`), which
unconditionally sets:

```perl
my $access_token_payload = {
    iss       => $self->get_issuer($req),
    exp       => $exp,
    aud       => $self->getAudiences($rp),
    client_id => $client_id,   # <-- always present, always the RP's client ID
    iat       => time,
    jti       => $id,
    scope     => $scope,
    sid       => $self->getSidFromSession( $rp, $sessionInfo ),
};
```

and then verified against a real minted, signature-checked token (decoded
payload from the final baked-config run):

```json
{
  "exp": 1783275804,
  "scope": "openid",
  "jti": "062cbba78c081e57a2d585c28c2aa59773b09ddcf245642d364a9b192f88485a",
  "sub": "apisix-portal-app",
  "iss": "http://auth.example.com",
  "client_id": "apisix-portal-app",
  "sid": "yysVAFdQDneW0kvUtzcE/OKBMS8uwQfwgiMeTyxgb7c",
  "iat": 1783272204,
  "aud": ["apisix-portal-app"]
}
```

Note that in this setup `sub` and `aud[0]` also happen to equal the
client ID (because the client-credentials grant handler seeds the "user"
identity with the client ID, and this RP has no additional audiences
configured) — but `client_id` is the one claim the LemonLDAP::NG source
guarantees will carry it, independent of scope/claims configuration, so
it is the correct, stable choice for `OIDC_CLIENT_ID_CLAIM`.

## How the config was produced

1. **Boot a throwaway container** to get a valid base config + directory
   layout:
   ```bash
   docker run -d --name lmtmp -p 19876:80 coudot/lemonldap-ng:2.20.2
   sleep 10
   docker cp lmtmp:/var/lib/lemonldap-ng/conf/lmConf-1.json /tmp/base.json
   ```
   The base config already ships `portal: http://auth.example.com/` and
   `locationRules` for `auth.example.com` / `manager.example.com`, so no
   domain edits were needed.

2. **Configure OIDC live, via `lemonldap-ng-cli`, not hand-edited JSON.**
   The image ships `/usr/share/lemonldap-ng/bin/lemonldap-ng-cli`, which
   talks to the same `File`-backed config store
   (`/var/lib/lemonldap-ng/conf`, per `/etc/lemonldap-ng/lemonldap-ng.ini`
   `[configuration]` section) that the running portal/manager use. This
   was safer than hand-editing the JSON because the CLI validates keys
   against LemonLDAP::NG's own attribute schema and rejects unknown/malformed
   config, and it recomputes internal invariants LL::NG expects
   (`cfgNum`, cache reload) automatically.

   ```bash
   CLI="docker exec lmtmp /usr/share/lemonldap-ng/bin/lemonldap-ng-cli -yes 1"
   $CLI set issuerDBOpenIDConnectActivation 1
   $CLI set oidcServiceMetaDataIssuer "http://auth.example.com"
   ```

3. **Generate the RSA signing key pair using LemonLDAP::NG's own key
   generator** (`Lemonldap::NG::Common::Util::Crypto::genRsaKey`, the same
   code the Manager UI's "generate new key" button calls under the hood via
   `POST /confs/newRSAKey`) rather than `openssl` directly, to guarantee
   the exact PEM flavour LL::NG expects:
   ```bash
   docker exec lmtmp perl -MLemonldap::NG::Common::Util::Crypto=genRsaKey -MJSON -e '
     print encode_json(genRsaKey(2048));
   ' > /tmp/rsakeys.json
   ```
   The resulting `private`/`public` PEM strings were set as
   `oidcServicePrivateKeySig` / `oidcServicePublicKeySig`, and the `hash`
   field (an md5_base64 of the public key) was set as `oidcServiceKeyIdSig`
   (used as the JWKS `kid`).

4. **Register the relying party** via `lemonldap-ng-cli merge` with a JSON
   fragment (nested-hash values aren't settable through plain `set`/`addKey`):
   ```json
   {
     "oidcRPMetaDataOptions": {
       "apisix-portal-app": {
         "oidcRPMetaDataOptionsClientID": "apisix-portal-app",
         "oidcRPMetaDataOptionsClientSecret": "apisix-portal-secret",
         "oidcRPMetaDataOptionsAllowClientCredentialsGrant": 1,
         "oidcRPMetaDataOptionsAccessTokenJWT": 1,
         "oidcRPMetaDataOptionsBypassConsent": 1,
         "oidcRPMetaDataOptionsIDTokenSignAlg": "RS256",
         "oidcRPMetaDataOptionsAccessTokenSignAlg": "RS256",
         "oidcRPMetaDataOptionsRedirectUris": "http://localhost/callback"
       }
     },
     "oidcRPMetaDataExportedVars": { "apisix-portal-app": {} },
     "oidcRPMetaDataScopes": { "apisix-portal-app": {} }
   }
   ```
   `oidcRPMetaDataOptionsRedirectUris` isn't required for the
   client-credentials grant itself, but LL::NG's config validator warns
   about RPs with no redirect URI at all; a harmless placeholder was added
   to keep the config warning-free.

5. **Export and bake** the final config via `lemonldap-ng-cli save`, then
   set the JSON's internal `cfgNum` field to `1` (LL::NG's `File` backend
   picks the "latest" config by scanning the directory for the highest
   `lmConf-<N>.json`, but it's cleaner/less surprising to keep the internal
   `cfgNum` field consistent with the filename it's baked into).

### Deviation from the brief: wrong config keys

The brief's suggested keys
`oidcRPMetaDataOptionsClientCredentials` do **not exist** in
LemonLDAP::NG 2.20's attribute schema (`Lemonldap::NG::Manager::Attributes`);
they are silently dropped by the config validator/merger (no error, key
just doesn't appear in the saved config — this is how the omission was
caught: `lemonldap-ng-cli save` didn't show the key after setting it).
The real attribute that gates the client-credentials grant is
**`oidcRPMetaDataOptionsAllowClientCredentialsGrant`** (bool), checked in
`Lemonldap::NG::Portal::Issuer::OpenIDConnect::_handleClientCredentialsGrant`.
This is the key actually baked into `lmConf-1.json`.

### Deviation: `scope=openid` is required on the token request

With an empty `scope` parameter, LemonLDAP::NG's `getScope()` resolves to
an empty scope string and the client-credentials handler returns
`{"error":"invalid_scope"}`. The token request must include
`-d scope=openid` (or another non-empty scope). This isn't a config
change — it just needs documenting for whoever calls the token endpoint
(Task 4 / test scripts).

### Deviation: baked config file must be world-readable

The container's PSGI/nginx workers run as `www-data` (uid 33), not the
host user. When `lmConf-1.json` was first `docker cp`'d out, it kept mode
`640` owned by the host user, and bind-mounting it read-only in a fresh
container gave `www-data` **no read access** at all (`Read error:
Permission denied` on every request, 500s on discovery/JWKS/token). The
file must be at least `644` on the host before mounting:
```bash
chmod 644 deploy/lemonldap/lmConf-1.json
```

### Harmless log noise: `sed: cannot rename ... Device or resource busy`

The image's `/docker-entrypoint.sh` unconditionally runs
`sed -i "s/example\.com/${SSODOMAIN}/" ... /var/lib/lemonldap-ng/conf/lmConf-1.json`
on every start. Since the config file is bind-mounted read-only from the
host, `sed -i`'s rename-over-mountpoint fails with `EBUSY` and this
warning is printed. It's a no-op anyway: `SSODOMAIN` defaults to
`example.com`, i.e. the substitution would replace `example.com` with
`example.com`. Verified the resulting config is untouched and everything
still works (see verification below) — this warning can be ignored.

## Verification performed

Fresh container, **only** the baked config mounted (matches how Task 4
will run it):
```bash
docker run -d --name lmtmp -p 80:80 \
  -v "$PWD/deploy/lemonldap/lmConf-1.json:/var/lib/lemonldap-ng/conf/lmConf-1.json:ro" \
  coudot/lemonldap-ng:2.20.2
```
`/etc/hosts` on this machine already maps `auth.example.com` /
`manager.example.com` to `127.0.0.1`.

1. **Discovery** (`GET /.well-known/openid-configuration`) → `HTTP 200`,
   `issuer=http://auth.example.com`, `jwks_uri` present,
   `grant_types_supported` includes `client_credentials`.
2. **JWKS** (`GET /oauth2/jwks`) → `HTTP 200`, one RSA key,
   `kid=yEYt3RiRb57U3GyC70u7cQ`.
3. **Token** (`POST /oauth2/token`, Basic auth
   `apisix-portal-app:apisix-portal-secret`,
   `grant_type=client_credentials&scope=openid`) → `HTTP 200`,
   `access_token` returned, `token_type=Bearer`.
4. **Decoded + signature-verified** the access token in Python (`PyJWT` +
   `cryptography`) against the JWKS public key, with `audience` and
   `issuer` checks — **signature valid**, payload as shown above.
5. **Negative test**: wrong `client_secret` → `HTTP 401`,
   `{"error":"invalid_client"}`.
6. Manager UI reachable at `http://manager.example.com/manager.html`
   (`HTTP 302`, redirects to login as expected).

All throwaway containers (`lmtmp`) were removed after verification
(`docker rm -f lmtmp`).

## Running it for real (Task 4 / full test stack)

```bash
docker run -d --name lemonldap -p 80:80 \
  -v "$PWD/deploy/lemonldap/lmConf-1.json:/var/lib/lemonldap-ng/conf/lmConf-1.json:ro" \
  coudot/lemonldap-ng:2.20.2
```
Point the portal at `OIDC_ISSUER=http://auth.example.com`,
`OIDC_CLIENT_ID_CLAIM=client_id`, and the credentials above. Requires
`auth.example.com` (and `manager.example.com`, for administering the IdP)
to resolve to wherever this container is published — via `/etc/hosts`,
container network alias, or (in a docker-compose test stack) a service
name alias.
