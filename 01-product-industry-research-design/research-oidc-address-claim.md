# Research: OIDC `address` claim members and the ROPC grant status

**AI tooling note:** Produced by Claude Code haiku research subagent via WebFetch/WebSearch against primary specifications.

## Question 1 — OIDC address claim

**Finding:** OpenID Connect Core 1.0, Section 5.1.1 "Address Claim" (https://openid.net/specs/openid-connect-core-1_0.html#AddressClaim) defines a JSON object containing these six optional members:

- `formatted` — full mailing address formatted for display or use on a mailing label
- `street_address` — street address component; **MAY contain multiple lines separated by newlines**
- `locality` — city or locality
- `region` — state, province, prefecture, or region
- `postal_code` — zip code or postal code
- `country` — country name

All members are **optional** — the entire address claim object is optional, and within it, no individual member is required.

**What the project had right/wrong:** Project rationale correctly named all six members with no omissions. The project did not list `formatted` in its initial statement ("we believe it is §5.1.1... with members formatted, street_address, locality, region, postal_code, country") but then immediately recognized `formatted` as a member we did not initially list — this was accurate self-correction. The note about `street_address` possibly containing newlines is confirmed; the spec states it "MAY contain multiple lines separated by newlines."

**Sources:**
- Spring Security AddressStandardClaim: https://docs.spring.io/spring-security/site/docs/current/api/org/springframework/security/oauth2/core/oidc/AddressStandardClaim.html (implements OIDC Core 1.0 with direct link to spec)
- oidc-client-ts OidcAddressClaim: https://authts.github.io/oidc-client-ts/interfaces/OidcAddressClaim.html
- OpenID Connect Core 1.0 specification: https://openid.net/specs/openid-connect-core-1_0.html#AddressClaim

## Question 2 — ROPC grant

**Finding:** RFC 6749, Section 4.3 defines the Resource Owner Password Credentials Grant: "The resource owner password credentials grant type is suitable in cases where the resource owner has a trust relationship with the client, such as the device operating system or a highly privileged application" (https://datatracker.ietf.org/doc/html/rfc6749#section-4.3).

OAuth 2.0 Security Best Current Practice (RFC 9700, Section 2.4) states: **"The resource owner password credentials grant [RFC6749] MUST NOT be used."** This is IETF normative language — the strongest prohibition.

**Sources:**
- RFC 6749 (OAuth 2.0 Authorization Framework), Section 4.3: https://datatracker.ietf.org/doc/html/rfc6749#section-4.3
- RFC 9700 (OAuth 2.0 Security Best Current Practice), Section 2.4: https://datatracker.ietf.org/doc/rfc9700/

## Could not verify

None — all claims confirmed from primary sources.
