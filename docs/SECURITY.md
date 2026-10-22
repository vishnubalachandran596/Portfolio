# Personal-data breach response checklist

Practical checklist for suspected or confirmed exposure of personal data on this site (Go app on Vercel, Supabase Postgres, Razorpay payments, Ed25519 licence signing key in the host env, stateless signed cookies with `token_version` revocation, Google sign-in). Read it with `ARCHITECTURE.md` (auth, env vars, deployment). Not legal advice: have a lawyer review it, and check the current DPDP Act and Rules before relying on it.

Work top to bottom, but contain first if exposure is ongoing. Write each action and time into the incident log (end of file) as you go.

## 1. Detect

- [ ] Vercel: runtime logs and runtime errors for the project (spikes of 401/403/500, unusual routes, odd IPs or user agents, admin routes hit).
- [ ] Supabase: Postgres, auth and API logs; unexpected roles, queries, or connections; advisor warnings (a table without RLS is a red flag).
- [ ] Razorpay: dashboard alerts, unexpected refunds/orders, webhook or signature failures.
- [ ] Reports: user or researcher email to the privacy contact, vishnubalachandran596@gmail.com. Acknowledge quickly and log the time received.
- [ ] Note first-seen time, who reported, and what was observed. Do not delete logs.

## 2. Contain

Do what the incident needs; each step has side effects (users signed out, downtime).

- [ ] Sessions: rotate `JWT_SECRET` in Vercel env and redeploy. Every signed cookie stops verifying. For targeted revocation, bump `token_version` in SQL for the affected users (`UPDATE users SET token_version = token_version + 1 WHERE ...`, or without the WHERE for everyone); the cookie carries it, so a bump signs them out everywhere. There is no admin button for a bulk bump.
- [ ] Admin: rotate `ADMIN_BOOTSTRAP_PASSWORD` and redeploy (the only admin password; nothing is stored in the DB). Review the `admins` table for unknown emails.
- [ ] Database: reset the Supabase DB password, update `DATABASE_URL` (Transaction pooler string) in Vercel, redeploy. Review roles and RLS.
- [ ] Payments: roll Razorpay `RAZORPAY_KEY_ID` / `RAZORPAY_KEY_SECRET` and any webhook secret. Payment confirmation stays server-side HMAC, so old keys must die.
- [ ] Google sign-in: rotate the OAuth client secret if it may have leaked.
- [ ] Licence signing key: generate a new pair at `/admin/licences/keygen`, set a NEW `LICENCE_KID` and `LICENCE_SIGNING_KEY_B64`. Add the new public key and kid to the Temple app's `config/licence_keys.json` and ship it BEFORE or WITH the switch. Keep the old public key listed so already-issued licences verify, unless the old key itself leaked (then licences must be reissued).
- [ ] Other secrets in the env (chatbot reindex token, storage keys, mail/embed keys): rotate any that could have been exposed.
- [ ] Access: revoke Vercel tokens, Git deploy keys and integrations; check Vercel/Supabase/GitHub team members and 2FA.
- [ ] Take the affected route or feature offline (firewall rule, redeploy without the route, or pause the project) if exposure continues.
- [ ] Preserve evidence (logs, a DB snapshot) before cleanup.

## 3. Assess

- [ ] What data: use the categories in the privacy policy (accounts and emails, password hashes, bookings, purchases and payment references, quiz/certificate records, licences and device IDs, contact messages, chatbot content). Razorpay holds card data, not this site.
- [ ] Whose and how many: customers, learners, booking guests, admins.
- [ ] Was it read, copied, altered, or only exposed? Is it still exposed? Were secrets involved (then treat step 2 as mandatory)?
- [ ] Likely harm to the people affected (phishing, account takeover, financial, identity).
- [ ] Root cause and entry point, even if provisional.

## 4. Notify

- [ ] Under India's Digital Personal Data Protection Act 2023 and its Rules, a personal data breach must be reported to the Data Protection Board of India and to each affected Data Principal, without delay, with the prescribed details, followed by a more detailed report within the prescribed time. Check the current Rules for exact timelines, form and contents.
- [ ] Affected users: plain-language message covering what happened, which data, likely consequences, what we did, what they should do (change reused passwords, watch for phishing), and the privacy contact. The site sends no email, so decide the channel (direct email from a personal mailbox, site banner, post).
- [ ] Inform Razorpay, Supabase, and Vercel where their systems or accounts are involved, and follow their incident processes.
- [ ] Consider police or CERT-In reporting where applicable; confirm obligations with counsel.
- [ ] Keep copies of everything sent and when.

## 5. Record and learn

- [ ] Complete the incident log below and keep it in a private place (not in this repo).
- [ ] Fix the root cause; add a regression test where code was at fault.
- [ ] Update `ARCHITECTURE.md` / `FUNCTIONALITY.md` if behaviour changed, and record the decision and lesson in Mem0.
- [ ] Review: what detected it, what was slow, which rotation steps were painful, and fix the runbook.

## Incident log template

```
Incident ID / title:
Detected (date, time, IST):         By / source:
Contained (date, time):             Notified Board / users (date, time):
Data categories:                    People affected (count, who):
Still exposed?  yes / no            Harm assessment:
Root cause:
Actions taken (time - action):
Secrets rotated (names only, never values):
Third parties informed:
Fix / test added:                   Docs and Mem0 updated:
Follow-ups and owners:
```
