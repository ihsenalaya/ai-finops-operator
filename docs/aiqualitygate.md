# CRD — AIQualityGate

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aiqgate`.

Valide si un modèle candidat est suffisamment sûr pour une application donnée. La granularité prévue
est **un `AIQualityGate` par application**.

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `target.namespace` / `target.application` | string | ✔ | Application protégée par le gate. |
| `target.team` | string | | Filtre optionnel si le même nom d'application existe dans plusieurs équipes. |
| `sourceModel` | string | ✔ | Modèle actuel. |
| `candidateModel` | string | ✔ | Modèle candidat. |
| `goldenDatasetRef.name/namespace/key` | ConfigMap ref | ✔ | Prompts métier de référence (`prompts.yaml` par défaut). |
| `evidenceRef.name/namespace/key` | ConfigMap ref | | Résultats déterministes d'un run golden (`results.yaml` par défaut), puis preuve auditable du score. Requis si des checks de réponse ou le score composite sont activés. |
| `gatewayRef` | string | | Gateway à utiliser pour la télémétrie. Par défaut, première `AIGateway` du namespace. |
| `period` | enum `daily\|weekly\|monthly` | | Fenêtre de télémétrie, défaut `monthly`. |
| `weights.correctness/reliability/latency/semantic/judged` | object | | Poids du score composite. Défaut `0.40/0.20/0.15/0.15/0.10`, renormalisés si le juge est désactivé. |
| `latencyThresholdMs` | int32 | | Seuil de latence utilisé par la dimension `Latency` (`seuil => 50`). |
| `minSamples` | int32 | | Nombre minimal de prompts golden requis. Défaut `1`. |
| `tolerancePoints` | int32 | | Tolérance de dégradation candidat vs source. Défaut `3`. |
| `judge.enabled` | bool | | Active la dimension `Judged` si un juge souverain est disponible. |
| `judge.modelRef` | string | | `AIModel` du juge LLM ; doit être conforme aux policies de souveraineté. |
| `evaluation.endpoint` | string | | Endpoint OpenAI-compatible `/v1/chat/completions` du data-plane de production appelé par le Job d'évaluation. |
| `evaluation.image` | string | | Image du Job d'évaluation. Défaut : image du pod opérateur en cours. |
| `evaluation.maxTokens` / `timeoutSeconds` | int32 | | Bornes par prompt pour les appels réels via gateway. |
| `requiredChecks.schemaValid` | bool | | Vérifie via evidence que le format attendu est respecté. |
| `requiredChecks.noUnexpectedRefusal` | bool | | Vérifie via evidence qu'il n'y a pas de refus inattendu. |
| `requiredChecks.noSensitiveDataLeak` | bool | | Vérifie via evidence qu'il n'y a pas de fuite sensible. |
| `requiredChecks.requiredKeywords[]` | []string | | Vérifie via evidence que les termes attendus sont présents. |
| `requiredChecks.maxErrorRatePercent` | int32 | | Seuil d'erreur observée du modèle candidat. |
| `requiredChecks.maxLatencyIncreasePercent` | int32 | | Seuil de dégradation latence candidat vs source. |
| `requiredChecks.maxRetryIncreasePercent` | int32 | | Réservé à la télémétrie retries. Si configuré aujourd'hui, le gate reste pending. |
| `canary.enabled/percent/duration` | object | | Contrat de canary attendu avant reroute complet. |
| `rollback.enabled/onErrorRatePercent/onLatencyIncreasePercent` | object | | Contrat de rollback attendu après actuation. |

## Status
`observedGeneration`, `phase` (`Pending|Passed|Failed`), `verdict`
(`insufficient-data|candidate-safe|candidate-risk`), `qualityScore`, `scoreBreakdown`,
`weightsUsed`, `samples`, `dimensions`, `checkedSamples`, `failedChecks`, `failureMessages[]`,
`canaryStatus`, `sourceObservation`, `candidateObservation`, `evaluationJobName`,
`evaluationJobPhase`, `conditions[]`.

## Comportement du controller
Le controller :

- lit et valide le golden dataset ;
- lance un `Job` Kubernetes d'évaluation si `evidenceRef` ne contient pas encore les sorties
  source/candidat ; ce Job appelle `spec.evaluation.endpoint` via le chemin gateway de production ;
- écrit les sorties réelles du Job dans l'evidence ConfigMap (`results.yaml`) ;
- lit l'evidence ConfigMap si des checks de réponse ou le score composite sont activés ;
- collecte la télémétrie via l'`AIGateway` ;
- calcule `qualityScore` via [qualityengine](../features/qualityscore.md) sans inventer de signal ;
- compare le score candidat au score source avec `tolerancePoints` ;
- expose `ai_finops_quality_score{namespace,app,provider,model,dimension}`,
  `ai_finops_quality_gate_passed` et `ai_finops_quality_gate_failed_checks` ;
- écrit un status lisible par un humain.

Sans golden dataset suffisant ou sans télémétrie réelle, le verdict reste `insufficient-data` et la
condition `Ready` porte `NoTelemetrySource` ou la raison de validation correspondante. Le canary de
trafic reste déclaratif tant que l'actuation complète n'est pas configurée.

## Exemple
[`..._aiqualitygate.yaml`](../../config/samples/aiops_v1alpha1_aiqualitygate.yaml).

```bash
kubectl apply -f config/samples/aiops_v1alpha1_aiqualitygate.yaml
kubectl get aiqgate finance-risk-assistant-quality-gate -o yaml
```
