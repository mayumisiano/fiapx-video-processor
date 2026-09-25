# 0003. Homegrown JWT + bcrypt, not a managed identity provider

Status: accepted

## Context

The brief requires the system to be protected by username and password. Candidates considered: Keycloak, Auth0, AWS Cognito, or implementing login/token issuance directly.

## Decision

Implement it directly: bcrypt for password hashing, a self-issued JWT for sessions. Identity is a generic subdomain — the problem is already solved industry-wide, so it's not where engineering effort should concentrate, but it's also small enough to build correctly by hand.

## Consequences

- No external dependency, no extra service to run, configure, or explain during the demo.
- JWT is stateless, so `videoprocessing` validates a request's identity without ever calling the identity code at runtime — only the token-issuing path is coupled to it.
- Demonstrates the security mechanics (hashing, signing, expiry) taught in the course, instead of hiding them behind a vendor.
- Trade-off accepted: no built-in password reset flow, MFA, or social login. Out of scope for this brief; would be a reason to switch to a managed provider if the product grew past the hackathon.
