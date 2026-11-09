# Glossary

Use one term consistently. The linked pages are the authoritative explanations.

| Term | Meaning |
|---|---|
| **SUT** | System under test. The host running the platform deployment. |
| **Load host** | The separate host running the generator, sink, capture and measurement tooling. |
| **B0** | The bounded ordinary workload: 25 msg/s, 1,000 devices, payload offset 0, N=2/k=2, CPU pool `4-5`, 300 s warm-up and 600 s measurement. |
| **W1–W4** | Separate fixed-point variants, five proposed attempts per platform each; not part of B0 and not automatically authorized. See [methodology](methodology.md#8-workload-configurations-b0-and-the-separate-w1w4-batch). |
| **TB / FW / EX** | ThingsBoard / FIWARE / EdgeX. |
| **QL** | FIWARE QuantumLeap history path. |
| **N** | Requested deployment replica count for the replicated application tier. |
| **k** | Application CPU budget in cores. B0 uses k=2 and pool `4-5`. |
| **HAProxy** | The common ingress path, present even at N=1, so replica-count changes do not change the ingress shape. |
| **benchctl** | The Go orchestration tool that runs the planned generator/capture workflow. |
| **k6 generator** | The open-loop HTTP workload generator. |
| **sink** | The external HTTP receiver that records complete-body delivery. |
| **receipt** | A sink record proving that a complete body was read. It is not proof of durable platform history. |
| **dispatch** | Application send after payload preparation. It is not physical wire transmission. |
| **`actual_send_ms`** | The application-dispatch timestamp used in the primary latency endpoint. |
| **Primary endpoint** | `receipt_complete_body_read_ms - actual_send_ms` under the dispatch contract. |
| **History** | Independently retrieved platform data used to reconcile identities after the run. |
| **Capture window** | The declared observation window, including the drain ceiling, quiet interval and late observation. |
| **Attempt** | One matrix entry with a unique run ID. Failed attempts remain in the ledger. |
| **Repetition** | One planned independent attempt for a platform/workload cell. A failure is not replaced automatically. |
| **Valid platform failure** | A valid experiment in which the platform failed or backlogged. It remains a result. |
| **Setup failure** | A pre-traffic failure in deployment, readiness, instrumentation or evidence. It is not a platform-performance result. |
| **`NOT_ASSESSED_NO_ANALYSIS`** | No measurement or analysis is supported because the workload was not launched or the evidence is insufficient. |
| **Logical padding** | The requested extra payload bytes before platform-specific serialization. |
| **Serialized body** | Platform-specific HTTP request body bytes after adapter encoding. |
| **Wire bytes** | Bytes actually observed on the network, including the declared protocol layers and direction. Header arithmetic is not a wire measurement. |
| **`dispatch-integrity-v1`** | The current workload-validity and dispatch-integrity policy. Send-lag p99 is diagnostic, not a standalone rejection rule. |

Canonical references: [architecture](architecture.md), [methodology](methodology.md),
[dispatch contract](methodology/dispatch-contract-20261106.md), and
[current status](status.md).
