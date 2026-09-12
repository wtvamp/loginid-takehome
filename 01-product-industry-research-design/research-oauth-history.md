# Research: a-auth.dev, Dick Hardt, and the OAuth Lineage

Ad hoc research note produced for the `01-product-industry-research-design` track, prompted by a mention of "a-auth.dev" and "Dick Hart" during the LoginID interview. Produced with Claude Code using WebSearch/WebFetch against public sources (Wikipedia, RFC Editor/IETF datatracker, project site, LinkedIn, podcast transcript). No file other than this one was modified.

## 0. First correction: the name and the domain

Two things said in the interview appear to be slightly off, and it's worth flagging before anything else:

- The person is **Dick Hardt**, not "Dick Hart." (There are unrelated people named "Dick Hart" — a footballer, a golfer — who show up in search results and are not this person.)
- The live project domain that resolves is **aauth.dev** (no hyphen). `a-auth.dev` returned `ENOTFOUND` (DNS does not resolve) when fetched directly during this research. It's likely a mishearing/mistyping of `aauth.dev` in conversation, or a domain that has since been retired/changed. This note treats `aauth.dev` as the intended site, based on content matching what was described in the interview.

## 1. What is aauth.dev?

Per the site's own content (fetched via WebFetch, 2026-09-12): AAuth is presented as an **Internet-Draft specification** — i.e. an IETF-track protocol proposal, not a shipping commercial product — with accompanying SDKs, tooling, and reference implementations, plus explainer/community material. It carries some manifesto-like advocacy language alongside the technical spec content, so it's fair to describe it as "a protocol proposal with a manifesto layer," rather than purely one or the other.

**Author:** Dick Hardt, described on the site as "original author of OAuth 2.0" (see Section 2 for how accurate that framing is). It's an open-source effort, developed in public with community contribution, and is explicitly positioned as going through (or aiming for) the IETF standards process rather than as a closed vendor spec.

**The problem it claims to solve:** traditional authorization protocols (OAuth 2.0 included) assume a client application is compiled/registered ahead of time, with a static, known relationship to the services it calls, and that a static, pre-registered client ID/secret model (API keys) is adequate. The site's argument is that AI agents break this assumption because they:

- assemble their own tool/service relationships dynamically at runtime, often against services never seen before,
- have no portable identity — an OAuth client ID registered with one provider doesn't transfer to another,
- rely on copying around long-lived API keys, which leak,
- need authorization decisions made mid-task rather than only as one upfront consent screen,
- operate across trust domains (multiple orgs, clouds, identity systems) simultaneously.

Quoted tagline from the site: *"Clients used to be written. Now agents assemble them at runtime. The protocols underneath weren't built for that,"* and *"The web gave servers identity. It's time clients got the same."*

**The proposed mechanism:** replace shared-secret API keys with **cryptographic agent identity** — every HTTP client gets its own verifiable identity (published at a well-known URL, verifiable without pre-registration or shared secrets) and signs its own requests directly (proof-of-possession per request) rather than presenting a bearer credential. The spec describes five progressive access modes (identity-based, resource-managed, person-identity, person-server, federated) and is explicitly framed as **coexisting with OAuth 2.0**, not replacing it — tokens carry identity + per-request authorization claims rather than standing, long-lived permissions.

**Uncertain/unconfirmed:** the deeper technical claims above (the five access modes, the specific token semantics) come from a single WebFetch summarization pass over the live site rather than a full read of the Internet-Draft text itself, and were not cross-checked against a second independent source (e.g., an actual IETF datatracker draft filing). Treat the mechanism description as "what the site says about itself," not independently verified protocol analysis.

## 2. Who is Dick Hardt, really, in OAuth history?

This is the part worth being careful about, because the interview framing ("associated with the original OAuth standard") is easy to over-read as "co-author of OAuth 1.0," and the evidence does not support that.

**Confirmed, from RFC 5849 (OAuth 1.0, published April 2010) itself:** the author/contributor list is Eran Hammer-Lahav (editor), Mark Atwood, Dirk Balfanz, Darren Bounds, Richard M. Conlan, Blaine Cook, Leah Culver, Breno de Medeiros, Brian Eaton, Kellan Elliott-McCrea, Larry Halff, Ben Laurie, Chris Messina, John Panzer, Sam Quigley, David Recordon, Eran Sandler, Jonathan Sergent, Todd Sieling, Brian Slesinsky, and Andy Smith. **Dick Hardt is not on this list.** He was not a co-author or credited contributor of OAuth 1.0 / RFC 5849. (Source: RFC Editor / IETF datatracker listing for RFC 5849.)

**What Dick Hardt actually did:** he was at Microsoft when OAuth 1.0 shipped, found it too complex to deploy, and — working with counterparts at Yahoo and Google — built an alternative simplified authorization profile originally intended to be called "OAuth" but renamed **OAuth WRAP** (Web Resource Authorization Profiles) after Eran Hammer-Lahav objected to reusing the OAuth name for something that didn't build on the OAuth 1.0 base. OAuth WRAP built on / was compatible with OAuth 1.0 concepts and was optimized for simpler implementation. Many WRAP concepts were subsequently folded into what became OAuth 2.0. (Source: Identity, Unlocked podcast transcript — "SignIn.org and the Genesis of the GNAP Working Group" — and IETF 77 (March 2010) OAuth WRAP slide deck.)

**His OAuth 2.0 role is well documented and real:** RFC 6749, "The OAuth 2.0 Authorization Framework," published October 2012, lists **D. Hardt (Ed.)** as editor. That is a genuine, verifiable, primary role — he shepherded and edited the specification that obsoleted OAuth 1.0. (Source: RFC 6749, rfc-editor.org.)

**Other confirmed biographical facts** (Wikipedia): born 1963, Canadian technology entrepreneur; founded ActiveState (sold to Sophos, 2003); a leading voice in the mid-2000s "Identity 2.0" movement; founded Sxip Identity (2003) and was a founding board member of the OpenID Foundation; joined Microsoft as a partner architect in December 2008 focused on identity, left January 2010 (this is the window during which the WRAP work happened); later founded Bubbler, joined Amazon in 2015. Wikipedia's biography text itself does not mention OAuth WRAP or AAuth by name — those details come from the RFC record and the podcast/site sources above, not from Wikipedia.

**Net assessment:** "associated with the original OAuth standard" is defensible only if read loosely — he is a central figure in the OAuth *lineage* (WRAP → OAuth 2.0 editor), but he was **not** a co-author of the original OAuth 1.0 standard (RFC 5849). The precise, defensible framing is: **"Dick Hardt is the editor of OAuth 2.0 (RFC 6749) and creator of OAuth WRAP, not a co-author of OAuth 1.0."** He also self-describes (LinkedIn) as "Creator of OAuth," which is a claim from his own bio and is the kind of self-characterization that should be flagged as his own framing rather than independently confirmed — the RFC record supports "OAuth 2.0 editor," which is a specific and significant credential, but the discrete OAuth 1.0 authorship claim doesn't hold up against RFC 5849's actual byline.

## 3. Broader OAuth timeline, for context

- **Nov 2006:** Blaine Cook builds an OpenID implementation for Twitter; around the same time Ma.gnolia needs a way to let Mac OS X Dashboard widgets access user accounts without sharing passwords. Cook, Chris Messina, and Larry Halff (Ma.gnolia), plus David Recordon, conclude there's no open standard for this kind of delegated API access.
- **April 2007:** an open OAuth discussion group forms; Google's DeWitt Clinton joins.
- **July 2007:** an initial spec draft exists; Eran Hammer-Lahav becomes the person coordinating the effort into a formal specification.
- **Oct 2007:** OAuth Core 1.0 final draft released.
- **Nov 2008:** IETF "Birds of a Feather" session at the Minneapolis meeting builds support for a formally chartered IETF OAuth working group.
- **~2009-2010:** Dick Hardt (at Microsoft) and colleagues at Yahoo/Google build OAuth WRAP as a simpler alternative/profile, intending it to fold back into the OAuth effort.
- **April 2010:** OAuth 1.0 published as **RFC 5849** (informational, not yet a full IETF standards-track document at that point).
- **Aug 2010:** Twitter requires all third-party apps to use OAuth.
- **Oct 2012:** **OAuth 2.0** published as **RFC 6749**, edited by Dick Hardt, obsoleting/replacing OAuth 1.0. (RFC 6750, Bearer Token Usage, published alongside it.)
- **Later:** the OAuth ecosystem continues to evolve (PKCE, device flow, token exchange, DPoP, GNAP as an even more ambitious successor effort that Hardt has also been involved with per the SignIn.org podcast source) — this track's research did not go deep into post-2012 OAuth evolution since it's outside what's needed to place a-auth.dev/AAuth in context.

## 4. Relevance to the LoginID assignment (Track 01 framing)

This track owns industry/competitive framing for the identity/IAM space LoginID operates in — passwordless auth, FIDO2/passkeys, and now AI-agent identity is an adjacent, fast-emerging sub-space worth naming explicitly in the framing document.

- **Complementary, not competing, with LoginID's core passkey/FIDO2 space.** AAuth is solving a different layer of the identity stack: it's about **authorizing and identifying HTTP clients/agents to services** (a client-credential / machine-to-machine authorization problem), whereas LoginID's core passwordless proposition is about **authenticating a human to a service** (replacing passwords with FIDO2/WebAuthn passkeys). The two are largely orthogonal: FIDO2 answers "is this the real person," AAuth-style protocols answer "is this the real agent/client, acting on whose behalf, with what scoped permission, for this one request." A mature identity platform plausibly needs both.
- **Where they do intersect:** AAuth's "person-identity" and "person-server" access modes (per the site's own framing) are explicitly about binding an agent's actions back to a human principal — which is squarely the kind of problem passkey-based strong authentication is meant to anchor. If LoginID were positioning itself for the agentic-AI moment, the credible story is "we provide the strong, phishing-resistant human-authentication anchor that agent-identity protocols like AAuth need at the root of their trust chain," not "we compete with AAuth."
- **For this take-home's actual assignment (DAO + REST API + third-party IDP connector):** this research is context/framing rather than something that changes the API contracts already specified by LoginID. It's useful supporting material for the industry-framing document (e.g., a paragraph situating this assignment's `/auth` + `/identity` connector pattern against the broader trend of moving away from static, pre-registered client credentials — the same critique AAuth levels at the "static client at compile-time" assumption applies loosely to why a third-party-IDP connector model, and multi-database credential storage, need to be designed with an eye toward evolving beyond simple username/password `user_credential` records).
- **Caveat for whoever uses this in the final submission:** don't cite "Dick Hart" or claim he co-authored OAuth 1.0 in the submission — use "Dick Hardt, editor of OAuth 2.0 (RFC 6749) and creator of OAuth WRAP" if a name/credential needs to be attached to a citation of AAuth/aauth.dev.

## Sources

- [aauth.dev](https://aauth.dev) — project site (fetched directly)
- [RFC 5849 — The OAuth 1.0 Protocol](https://www.rfc-editor.org/rfc/rfc5849.html) (RFC Editor)
- [RFC 5849 info page / author list](https://www.rfc-editor.org/info/rfc5849/)
- [RFC 5849 — IETF Datatracker](https://datatracker.ietf.org/doc/rfc5849/)
- [RFC 6749 — The OAuth 2.0 Authorization Framework](https://www.rfc-editor.org/rfc/rfc6749.html) (RFC Editor)
- [RFC 6749 info page](https://www.rfc-editor.org/info/rfc6749/)
- [OAuth — Wikipedia](https://en.wikipedia.org/wiki/OAuth)
- [Dick Hardt — Wikipedia](https://en.wikipedia.org/wiki/Dick_Hardt)
- [Dick Hardt — LinkedIn ("Creator of OAuth, building AAuth")](https://pt.linkedin.com/in/dickhardt) — self-description, treated as a claim rather than independent confirmation
- [OAuth WRAP Overview, Dick Hardt, IETF 77 slide deck, March 22, 2010](https://www.ietf.org/proceedings/77/slides/oauth-1.pdf)
- [SignIn.org and the Genesis of the GNAP Working Group — Identity, Unlocked podcast transcript](https://identityunlocked.auth0.com/public/49/Identity,-Unlocked.--bed7fada/3a164a46)
- [OAuth Web Resource Authorization Profiles (OAuth WRAP) — Microsoft Learn (archived docs)](https://learn.microsoft.com/en-us/previous-versions/azure/azure-services/hh801906(v=azure.100))
