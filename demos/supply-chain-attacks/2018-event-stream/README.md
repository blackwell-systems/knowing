# event-stream (November 2018)

## Summary

An attacker gained maintainer access to event-stream (2M weekly npm downloads),
added a dependency on `flatmap-stream`, which contained obfuscated code targeting
the Copay Bitcoin wallet. Private keys were exfiltrated via HTTPS when the malicious
code detected the wallet library in the dependency tree.

The attack went undetected for 2 months despite massive download volume.

## Attack Vector

Social engineering:
1. Attacker offered to maintain the abandoned event-stream package
2. Original maintainer transferred ownership
3. Attacker added `flatmap-stream` as a dependency (v3.3.6)
4. `flatmap-stream` contained obfuscated code that activated only when `copay-dash`
   (a Bitcoin wallet library) was present in the dependency tree
5. When activated: decrypt payload, steal wallet private keys, exfiltrate via HTTPS

## Payload

- `flatmap-stream/index.js`: obfuscated with `uglify-js`, AES-encrypted inner payload
- Activation condition: `require.resolve('copay-dash')` succeeds
- Decrypts inner code using a key derived from the copay-dash package description
- Steals Bitcoin wallet private keys
- Exfiltrates to `copay.io` domain via HTTPS POST

## Detection by knowing

**Structural signals:**
- New dependency (`flatmap-stream`) not present in v3.3.3
- `flatmap-stream` has capability paths to `crypto.createDecipher` and `https.request`
- event-stream v3.3.3 has NO path to any network API (prove-absent succeeds)
- event-stream v3.3.6 gains a transitive path: flatmap-stream -> crypto -> https
- Isolation score: high (flatmap-stream has 1 inbound edge from event-stream,
  outbound edges to crypto + network)

**What knowing proves:**
- Clean version (v3.3.3): event-stream is capability-isolated from network APIs
- Compromised version (v3.3.6): new capability path exists from flatmap-stream to https.request
- The exact edge that enables the attack is identifiable in the diff
- Proof is cryptographically verifiable offline

## Why This Matters

This attack demonstrates the limitations of existing defenses:
- npm audit: no CVE existed (novel attack, 0-day)
- Signature verification: the publish was legitimate (attacker had credentials)
- Lockfile pinning: v3.3.6 was the latest when users installed
- Code review: obfuscation hid the payload from casual inspection

knowing's structural analysis would have detected it at publish time:
- New dependency adds a path to `crypto` + `https` that didn't exist before
- `prove-absent` fails immediately on the compromised version
- CI gate blocks the PR that adds the dependency

## Timeline

- 2018-09: Attacker gains maintainer access to event-stream
- 2018-09-16: v3.3.6 published with flatmap-stream dependency
- 2018-11-20: Community discovers the attack (2 months later)
- 2018-11-26: flatmap-stream unpublished from npm

## References

- Original disclosure: https://github.com/dominictarr/event-stream/issues/116
- npm blog: https://blog.npmjs.org/post/180565383195/details-about-the-event-stream-incident
- Snyk analysis: https://snyk.io/blog/a-post-mortem-of-the-malicious-event-stream-backdoor/
