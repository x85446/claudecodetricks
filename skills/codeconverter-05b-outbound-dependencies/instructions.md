<!-- NATIVE to the codeconverter pipeline — no legacy codeplanner ancestor. -->

> **Stage note — read first.** Unlike stages 01–11, this stage was not adapted from
> the legacy "codeplanner" process; that process had no outbound-dependency phase.
> There is no phase-number translation table to apply and no journey.md/journal in
> this pipeline. Where this document conflicts with the stage's SKILL.md output
> contract (uniform headers, MANIFEST.md, output directory), **SKILL.md wins**.

---

# Stage 05b — Outbound Dependency Map

## Mission

Enumerate everything the source service calls **out** to, so the replacement can be
built knowing exactly what it must still be able to reach.

Stage 05 answers "what can be called on this service". Stage 05a answers "who calls
it". Stage 09 answers "who couples to its internals". None of them answers "what does
it need on the other end of the wire". That is this stage.

The output is a checklist a rewrite is graded against: every row is something the
replacement must be able to connect to, authenticate against, and speak the protocol
of, on day one.

---

## Why the categories are named (keep this in view while working)

An audit told to find "external calls" finds the interesting half and stops. The
categories below exist so that skipping one is a visible omission rather than a
judgement call. Each gets a section in the output whether or not anything is found,
and a "none found" section must show the searches that justify it.

| # | Category | What it covers |
|---|---|---|
| 1 | **HTTP clients** | outbound REST/gRPC/SOAP to other services, internal or third-party |
| 2 | **Database** | every SQL/NoSQL store the service connects to, including read replicas and migration runners |
| 3 | **Cache** | Redis/Memcached/Hazelcast, session and token stores, distributed locks |
| 4 | **Message bus** | RabbitMQ/Kafka/NATS/SQS — every exchange, topic and queue, in both directions |
| 5 | **Object store** | S3 and compatible, blob/file storage, uploads and static assets |
| 6 | **SMTP / email** | mail transport, and any templated-email or transactional-email provider |
| 7 | **Licensing** | license server or entitlement service calls, seat/feature checks |
| 8 | **Billing** | billing/metering/usage-reporting clients |
| 9 | Unclassified sweep | anything opening a socket that lands in none of the above (LDAP, DNS, NTP, HSM, KMS, OTel/metrics exporters, feature-flag services, webhooks) |

Category 9 exists so that "not one of the eight" cannot become "not recorded". Move
anything it catches into a named category if one fits, and leave it in 9 with a name
if none does.

### Calibration floor

Before declaring done, check the findings against the infrastructure the service
demonstrably runs against — STATE.md's environment notes and service ports, the
source repo's `docker-compose*.yml` / Helm charts / deployment manifests, and the
dependency manifest. Anything present there and absent from the findings is either a
miss or needs a written explanation.

For the IAM `auth` source service the floor is known: **PostgreSQL, Redis, RabbitMQ
(exchanges `iam_accounts`, `iam_apikeys`, `iam_applications`, `iam_certificates`),
S3, SMTP, a license server, and a billing client.** A run against `auth` that
reports fewer than these is incomplete, whatever its exit criteria say. Treat that
list as a worked example of what a calibration floor looks like, and derive the
equivalent list for any other source service from its own compose and deployment
files.

---

## Step 0 — Readiness

```bash
# Dependency manifests — the map of what client libraries are even available
ls pom.xml go.mod package.json requirements.txt build.gradle 2>/dev/null
find . -maxdepth 3 -name 'pom.xml' -o -maxdepth 3 -name 'go.mod' | head -20

# Deployment/compose files — what the service is wired to at runtime
find . -maxdepth 3 \( -name 'docker-compose*.y*ml' -o -name 'values*.y*ml' \
  -o -name '*.tf' -o -path '*charts*' -name '*.yaml' \) | head -40

# Config surface
find . -name '*.properties' -o -name 'application*.y*ml' -o -name '*.conf' | head -40

# The deployment manifests are often NOT in the source repo.  STATE.md names where
# they are; look there too, or the config half of every finding will be missing.
grep -E "Deployment manifests path" docs/codeconverter/STATE.md
ls {DEPLOYMENT_MANIFESTS_PATH}

# What prior stages already believe
ls docs/codeconverter/02-codebase-analysis/
grep -n "Service ports\|Environment notes" -A 20 docs/codeconverter/STATE.md | head -60
```

Record this in a "Readiness" section of `outbound-dependencies.md`. The dependency
manifest and the compose files are your two independent cross-checks; capture them
now, before grep biases what you look for.

**If the source repo has no Helm chart or Terraform, that is not "no deployment
config" — it means the deployment config lives in the manifests repo STATE.md names.**
On the IAM run the SMTP host, port, protocol and sender address were nowhere in the
source repo; they arrive from a `platform-email-server` ConfigMap in a sibling repo,
via Helm values, into a system property the code reads. Reporting "SMTP: hardcoded" or
"SMTP: no config key" would have been wrong on both counts.

---

## Step 1 — Inventory client libraries from the dependency manifest

Every outbound call needs a client. List them first, then go find their call sites.

```bash
# Java
grep -nE '<artifactId>' pom.xml */pom.xml | grep -iE \
  'http|client|jdbc|postgres|mysql|redis|jedis|lettuce|rabbit|amqp|kafka|aws|s3|mail|smtp|javamail|grpc|feign|resteasy|jersey|okhttp'

# Go
grep -nE 'redis|amqp|pq|pgx|aws-sdk|minio|smtp|grpc|resty|sarama' go.mod

# Node
grep -nE '"(axios|node-fetch|got|superagent|pg|mysql2|ioredis|redis|amqplib|kafkajs|aws-sdk|@aws-sdk|nodemailer)"' package.json

# Python
grep -niE 'requests|httpx|psycopg|sqlalchemy|redis|pika|kombu|boto3|smtplib' requirements.txt
```

Produce a table: library → category guess → "call sites found?" (filled in later).
A library on this list with no call site by the end is a finding in itself —
either dead, or a call site you have not found yet. Say which.

---

## Step 2 — Category sweeps

Run every sweep. Adapt the patterns to the source language; the categories do not
change.

### Sweep 0 — find the abstractions by name, before grepping for protocols

Production code hides its outbound calls behind domain-named interfaces whose names
contain none of the protocol keywords. In the IAM `auth` source, the SMTP path is
`iam-common/.../email/client/SimpleEmailService.java` — a protocol-keyword grep finds
`JavaMailSender` inside it, but a *config* grep for `smtp` finds nothing anywhere,
and a reader skimming class names for "SMTP" finds nothing either.

So sweep the names first, and use the results to target the protocol greps:

```bash
# Classes/files named after each category
find . -path '*/main/*' \( -iname '*client*' -o -iname '*service*' -o -iname '*sender*' \
  -o -iname '*publisher*' -o -iname '*repository*' -o -iname '*dao*' -o -iname '*gateway*' \) \
  -not -path '*/target/*' | head -60

# The category nouns, in type and package names
grep -rniE 'class [A-Za-z]*(Email|Mail|Notification|Billing|Licen|Message ?bus|Storage|Cache|Repository)' \
  --include='*.java' --include='*.go' --include='*.py' . | grep -v '/target/' | head -40
```

Then, for each abstraction found, read its implementation and record which category
its transport belongs to. An interface with two implementations (a real one and a
mock/local one) is a finding about the real one — note the mock separately, and
never let the existence of a `LocalMessageQueue` or `LicensingMock` stand in for the
production dependency.

### 1. HTTP clients (outbound REST / gRPC)

```bash
# Java
grep -rn 'HttpClient\|WebClient\|RestTemplate\|OkHttpClient\|ClientBuilder\.newClient\|WebTarget\|Feign\|WebClient\.create' --include='*.java' src */src | grep -v import
# Vert.x
grep -rn 'createHttpClient\|WebClient\.create\|\.requestAbs(\|\.getAbs(\|\.postAbs(' --include='*.java' .
# Go / Node / Python
grep -rn 'http\.NewRequest\|http\.Client{\|grpc\.Dial' --include='*.go' .
grep -rnE 'axios\.|fetch\(|got\(' --include='*.ts' --include='*.js' .
grep -rnE 'requests\.(get|post|put|delete)|httpx\.' --include='*.py' .

# The URLs and hostnames they point at — often the only way to identify the target
grep -rnE 'https?://[a-zA-Z0-9._-]+' --include='*.properties' --include='*.y*ml' --include='*.java' . \
  | grep -vE 'w3\.org|xmlns|schema|apache\.org|example\.com'
grep -rnE '\.svc\.cluster\.local|_HOST|_URL|_ENDPOINT|_BASE_URI' --include='*.y*ml' --include='*.properties' .
```

For each: which service is on the other end, what it is called for, whether it is on
the synchronous request path (a customer request blocks on it) or background, and
what the timeout/retry behavior is. Note internal calls between the source service's
own modules separately — they look like in-process work in a rewrite but are network
hops today, with network failure modes.

### 2. Database

```bash
grep -rnE 'jdbc:|DriverManager|DataSource|HikariConfig|EntityManager|@Entity|SessionFactory' --include='*.java' . | grep -v import
grep -rnE 'sql\.Open|pgxpool|gorm\.Open' --include='*.go' .
grep -rnE 'jdbc:|POSTGRES|PGHOST|PGDATABASE|DB_HOST|DB_NAME|datasource' --include='*.properties' --include='*.y*ml' --include='*.env' .

# Schema and migrations — the shape the replacement must keep
find . -path '*migration*' -name '*.sql' -o -name 'V*__*.sql' -o -path '*flyway*' -o -path '*liquibase*' | head -40
```

Record: engine and version, database name(s), user(s), connection pool settings,
whether migrations run in-process at startup, read/write split, and the tables (or a
reference to stage 02's storage map, verified — do not re-list 50 tables here).

### 3. Cache

```bash
grep -rnE 'Jedis|Lettuce|RedisClient|RedisTemplate|Redisson|Memcached|Hazelcast' --include='*.java' . | grep -v import
grep -rnE 'redis\.|go-redis|memcache' --include='*.go' --include='*.py' --include='*.ts' .
grep -rnE 'REDIS|redis:|cache\.host|cache\.port' --include='*.properties' --include='*.y*ml' .
```

Record: engine, what is cached (sessions, tokens, rate-limit counters, distributed
locks), the **key namespaces/prefixes**, TTLs, and whether the cache is
authoritative for anything (a token store is state, not cache — losing it logs
everyone out). Cache key namespaces matter to the rewrite: a replacement running
side by side with the original must agree on them or must not share the instance.

### 4. Message bus

```bash
grep -rnE 'ConnectionFactory|Channel\.|basicPublish|basicConsume|exchangeDeclare|queueDeclare|@RabbitListener|AmqpTemplate|RabbitTemplate' --include='*.java' . | grep -v import
grep -rnE 'KafkaProducer|KafkaConsumer|@KafkaListener|sarama\.|nats\.Connect|sqs\.' --include='*.java' --include='*.go' .

# The names themselves — exchanges, topics, queues, routing keys
grep -rnE '"[a-z0-9_.-]*(exchange|queue|topic|routing)[a-z0-9_.-]*"' --include='*.java' --include='*.properties' --include='*.y*ml' .
grep -rnE 'AMQP|RABBIT|BROKER_URL|KAFKA' --include='*.properties' --include='*.y*ml' --include='*.env' .
```

Record every exchange/topic/queue **individually**, with: exact name, type (direct/
topic/fanout), direction (publish/consume/both), routing keys, message schema
summary, durability, and the consumer on the other end if known. "The service
publishes events" is not a finding. `iam_accounts` (topic, publish, routing key
`account.created`, consumed by `device-directory`) is a finding.

Cross-check against `05-api-surface/API.md` section 5, which lists the message bus
from the inbound perspective — the two must agree, and any disagreement is a finding.

### 5. Object store

```bash
grep -rnE 'AmazonS3|S3Client|MinioClient|PutObjectRequest|GetObjectRequest|getObject\(|putObject\(' --include='*.java' --include='*.go' --include='*.py' . | grep -v import
grep -rnE 'S3_|AWS_|BUCKET|ENDPOINT_URL|s3\.' --include='*.properties' --include='*.y*ml' --include='*.env' .
```

Record: buckets, key prefixes, what is stored (branding images, certificates,
exports), whether the endpoint is real AWS or an S3-compatible mock/on-prem
(`s3mock`, MinIO), path-style vs virtual-host addressing, and credential source.

### 6. SMTP / email

```bash
grep -rnE 'javax\.mail|jakarta\.mail|MimeMessage|Transport\.send|JavaMailSender|smtplib|nodemailer|SendGrid|Mailgun|SES' --include='*.java' --include='*.py' --include='*.ts' --include='*.go' . | grep -v import
grep -rniE 'smtp|mail\.host|mail\.port|MAILER|FROM_ADDRESS|EMAIL_' --include='*.properties' --include='*.y*ml' --include='*.env' .

# Name-based (Sweep 0 applied here) — this is the pattern that finds an SMTP path
# whose class name never says "smtp"
find . -path '*/main/*' -iname '*mail*' -o -path '*/main/*' -iname '*email*' | grep -v '/target/'
grep -rnE 'class [A-Za-z]*(Email|Mail)[A-Za-z]*(Service|Sender|Client|Notification)' --include='*.java' . | grep -v '/target/'

# Email templates are part of the contract — a rewrite that drops one breaks a flow
find . -path '*template*' \( -name '*.html' -o -name '*.ftl' -o -name '*.vm' -o -name '*.mustache' \) | head -40
```

Record: transport (direct SMTP vs provider API), host/port/TLS mode, sender
identities, and **every template and the flow that sends it** (invitation, password
reset, verification, expiry warning). Templates are behavior the replacement must
reproduce.

### 7. Licensing

```bash
grep -rniE 'licen[cs]e|entitlement|seat|activation' --include='*.java' --include='*.go' --include='*.py' . | grep -viE 'license header|apache licen|MIT licen|SPDX|LICENSE\.txt' | head -60
grep -rniE 'LICENSE_(URL|HOST|SERVER)|licensing\.' --include='*.properties' --include='*.y*ml' --include='*.env' .
```

The noise filter matters — license *headers* are in every source file. Filter for
runtime calls: a client class, a URL, an endpoint path. Record: which service,
which endpoints are called, what the service does when licensing is unreachable
(fail open or fail closed — this is a behavior the replacement must match exactly),
and what it gates.

### 8. Billing

```bash
grep -rniE 'billing|metering|usage.?report|invoice|subscription|quota' --include='*.java' --include='*.go' --include='*.py' . | grep -v import | head -60
grep -rniE 'BILLING_(URL|HOST)|billing\.' --include='*.properties' --include='*.y*ml' --include='*.env' .
```

Record: which billing service, what is reported and on what trigger (per-request,
scheduled, on lifecycle events), the payload shape, and whether a failed report is
retried or dropped. Under-reporting is a silent revenue bug — note the failure mode.

### 9. Unclassified sweep

```bash
# Anything else opening a connection
grep -rnE 'new Socket|SSLSocket|DatagramSocket|InetAddress\.getBy|net\.Dial|LdapContext|InitialDirContext|DirContext' --include='*.java' --include='*.go' . | grep -v import
grep -rniE 'ldap|kerberos|vault|kms|hsm|pkcs11|otlp|opentelemetry|statsd|prometheus|jaeger|zipkin|webhook|feature.?flag' \
  --include='*.java' --include='*.properties' --include='*.y*ml' . | head -60
```

Telemetry exporters count. A replacement that cannot reach the collector is not
broken for customers but is broken for operations, and that belongs on the list with
its severity set accordingly.

---

## Step 3 — Verify every candidate

Grep produces candidates, not findings. For each:

1. **Read the file.** Confirm it is a live call path, not dead code, not a test-only
   fixture, not a commented-out block.
2. **Find both halves.** Code that calls, and the setting that points it at a
   target. A finding with only one half is incomplete: code alone does not say where
   it points, config alone does not prove anything reads it.

   The setting is often **not** a checked-in `.properties` or `.yaml` key. It may be
   a constant in a configuration class, a getter on a Kubernetes config object, a
   Helm value, or an environment variable that exists only in the deployment
   manifest. In the IAM `auth` source, the email host arrives through
   `SimpleEmailServiceConfig` ← `SharedConfiguration` ← a `kubeConfig` getter, and
   grepping `*.properties` for `smtp` returns nothing at all. Follow the accessor
   chain to whatever actually supplies the value, and record that as
   `config_source`. Only when the chain genuinely ends without a settable key —
   a hardcoded host — do you record `config_keys: []`, and then say "hardcoded" and
   quote the line, because a hardcoded target is itself a migration finding.
3. **Trace the credential.** Where does the secret come from — env var, mounted
   K8s secret, vault lookup, config file? The replacement needs the same one.
4. **Decide request-path or not.** Does a customer HTTP request block on this call?
   That determines the severity of an outage of the dependency.
5. **Mark test-only dependencies as such** and keep them: the replacement's test
   environment needs them too, but they are not production dependencies.

---

## Step 4 — Write the outputs

### `outbound-dependencies.md`

```markdown
<standard artifact header block>

# <Service> — Outbound Dependency Map

## Readiness
<Step 0 output: manifests, compose files, config surface found>

## Summary

| Category | Findings | On request path | Critical |
|---|---:|---:|---:|
| HTTP clients | | | |
| Database | | | |
| Cache | | | |
| Message bus | | | |
| Object store | | | |
| SMTP / email | | | |
| Licensing | | | |
| Billing | | | |
| Unclassified | | | |
| **Total** | | | |

## Client library inventory
<Step 1 table: library -> category -> call sites found / dead>

## Calibration check
<infrastructure named in STATE.md + compose/deployment manifests, each mapped to a
 finding, or explained>

## 1. HTTP clients
### OUT-001 — <target name>
| Field | Value |
|---|---|
| Category | http-client |
| Target | <service / host> |
| Protocol | HTTPS / gRPC / AMQP / ... |
| Direction | outbound-call / publish / consume / read-write |
| Evidence | `path/File.java:88` |
| Snippet | `client.get(licenseUrl + "/v1/check")` |
| Config keys | `LICENSE_SERVER_URL`, `license.timeout.ms` |
| Config source | env var / properties file / config class / Helm value / K8s secret / hardcoded |
| Credential | K8s secret `iam.license.token`, mounted at ... |
| Request path | yes / no |
| Failure mode | fail-open / fail-closed / retried / dropped |
| What breaks | <specific consequence for the replacement> |
| Criticality | critical / high / medium / low |

<repeat per finding; one section per category; "none found" sections carry the
 searches that prove it>

...

## 9. Unclassified

## Findings with no config key / no call site
<the loose ends, named rather than dropped>
```

### `outbound-dependencies.json`

```json
{
  "stage": "05b-outbound-dependencies",
  "generated": "YYYY-MM-DD",
  "source_service": "<name>",
  "categories": [
    "http-client", "database", "cache", "message-bus", "object-store",
    "smtp", "licensing", "billing", "unclassified"
  ],
  "findings": [
    {
      "id": "OUT-014",
      "category": "message-bus",
      "target": "RabbitMQ exchange iam_accounts",
      "protocol": "AMQP 0-9-1",
      "direction": "publish",
      "detail": {"exchange_type": "topic", "routing_keys": ["account.created", "account.updated"], "durable": true},
      "evidence": [{"file": "iam-identity/src/main/java/.../AccountEventPublisher.java", "line": 61,
                    "snippet": "channel.basicPublish(\"iam_accounts\", routingKey, props, body)"}],
      "config_keys": ["RABBITMQ_HOST", "rabbitmq.exchange.accounts"],
      "config_source": "env var, read via MessagebusConfigResolver",
      "credential": "env RABBITMQ_USER / RABBITMQ_PASSWORD",
      "request_path": false,
      "failure_mode": "logged and dropped",
      "what_breaks": "downstream services stop seeing account lifecycle events",
      "criticality": "critical",
      "test_only": false
    }
  ],
  "summary": {
    "total": 0,
    "by_category": {},
    "by_criticality": {},
    "on_request_path": 0,
    "test_only": 0
  },
  "calibration": {
    "expected": ["postgresql", "redis", "rabbitmq", "s3", "smtp", "license-server", "billing"],
    "found": [],
    "unexplained_absences": []
  }
}
```

Rules for the JSON:

- Every finding in the markdown appears in the JSON and vice versa; ids match.
- `summary.by_category` covers all nine categories, zeros included.
- `calibration.unexplained_absences` must be empty to declare the stage complete.

---

## Verification before you declare done

Grep-derived line numbers drift. Before anything else, open every cited `file:line`
and confirm the recorded snippet is on it. Do this mechanically, over all of them —
a hand check of five citations out of eighty will not find the two that are wrong:

```python
# for each finding, for each evidence entry: read the source line at e['line'] and
# require a distinctive fragment of e['snippet'] (a quoted literal, or a long
# identifier) to appear within that line and the two below it.
# Report "<n> verified, <m> failed" and fix every failure before declaring done.
```

This is not a re-grep of your own output — it reads the source. On the IAM run it
found 4 wrong line numbers out of 83, every one of them in a JSON config file where
the number had been estimated rather than read.

```bash
python3 - <<'PY'
import json
d = json.load(open('docs/codeconverter/05b-outbound-dependencies/outbound-dependencies.json'))
f = d['findings']
cats = set(d['categories'])
assert len(f) == d['summary']['total'], "total mismatch"
assert set(d['summary']['by_category']) == cats, "by_category must cover all 9 categories"
for x in f:
    assert x['category'] in cats, x['id']
    assert x['evidence'] and all(e.get('file') and e.get('line') for e in x['evidence']), x['id']
    assert x.get('what_breaks') and x.get('criticality'), x['id']
assert not d['calibration']['unexplained_absences'], d['calibration']['unexplained_absences']
print(len(f), "findings OK across", len(cats), "categories")
PY

# Every category has a section in the markdown (none-found included)
for c in "HTTP clients" "Database" "Cache" "Message bus" "Object store" \
         "SMTP" "Licensing" "Billing" "Unclassified"; do
  printf '%-15s %s\n' "$c" \
    "$(grep -c "$c" docs/codeconverter/05b-outbound-dependencies/outbound-dependencies.md)"
done

# ID counts agree between the two artifacts
grep -oE 'OUT-[0-9]+' docs/codeconverter/05b-outbound-dependencies/outbound-dependencies.md | sort -u | wc -l
```

---

## Exit Criteria

Stage 05b is complete when:

- [ ] All eight named categories plus the unclassified sweep were searched; every
      one has a section, and "none found" sections show the searches that prove it.
- [ ] Every finding carries `file:line` evidence with a quoted snippet, and pairs
      code with the config key that points it at a target.
- [ ] Every cited `file:line` was opened and the snippet confirmed to be there;
      the verified/failed counts are recorded.
- [ ] Every finding records protocol, target identity, config keys, credential
      source, direction, request-path yes/no, failure mode, what breaks, and
      criticality.
- [ ] The client library inventory is complete, and every library is either mapped to
      a call site or explicitly declared dead.
- [ ] Message-bus findings name every exchange/topic/queue individually with type,
      direction and routing keys, and agree with `05-api-surface/API.md` section 5.
- [ ] The calibration check passed: `calibration.unexplained_absences` is empty.
- [ ] Markdown and JSON counts match and the checking command is shown.
- [ ] `MANIFEST.md` exists, matches the template, and every exit criterion above is
      copied into it and honestly checked.
