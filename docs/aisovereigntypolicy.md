# CRD — AISovereigntyPolicy

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aisov`.

Définit les règles de résidence/souveraineté des données et **pilote l'enforcement** via
`enforcementMode`. Ne constitue pas une attestation juridique.

## Spec
| Champ | Type | Description |
|-------|------|-------------|
| `dataResidency.allowedZones[]` | []string | Zones autorisées (ex. `FR`, `EU`). |
| `dataResidency.forbiddenZones[]` | []string | Zones interdites (ex. `US`, `CN`). |
| `sensitiveData.externalProvidersAllowed` | bool | Donnée sensible vers fournisseurs externes. |
| `sensitiveData.requireAnonymization` | bool | Anonymisation requise. |
| `audit.retainLogsDays` | int32 | Rétention attendue des logs d'audit. |
| `audit.immutableLogs` | bool | Logs inviolables attendus. |
| `enforcementMode` | enum `reportOnly\|warn\|enforce` (défaut `reportOnly`) | Réaction aux violations (voir ci-dessous). |

## Modes d'enforcement
Sur chaque réconciliation, les findings **critiques** (zone interdite) sont transformés en décisions par
[`enforcementengine`](../features/enforcementengine.md) :

| Mode | Action | Effet |
|------|--------|-------|
| `reportOnly` | `report` | Constat seul, aucune action (défaut). |
| `warn` | `warn` | **Alerte différenciée** : Event Kubernetes `Warning`/`Enforcement` + série `ai_finops_enforcement_actions{action="warn",actuated="true"}`. Ne bloque pas. |
| `enforce` | `reroute` ou `block` | **Actuation réelle** dans Envoy AI Gateway : reroute vers le backend conforme (+ réécriture du `model`) quand un fallback existe, sinon blocage via le backend réservé absent `aiops-blocked`. `actuated="true"` uniquement si la route est patchée, réversible (annotation + finalizer). |

## Status
`observedGeneration`, `findingsCount`, `conditions[]` (`Ready`, dont le message résume la posture
d'enforcement : `… enforcement: N warn, N reroute, N block`).

## Comportement du controller
Collecte l'usage, déduit les fournisseurs employés, évalue via
[sovereigntyengine](../features/sovereigntyengine.md) :
- zone interdite ⇒ **critical** ;
- hors zones autorisées ⇒ **warning** (EU couvre les pays UE) ;
- fournisseur managé externe alors que `externalProvidersAllowed=false` ⇒ **warning** ;
- `requireAnonymization` ⇒ **info**.

Renseigne `findingsCount`, expose `ai_finops_sovereignty_findings{severity}` et, selon le mode,
`ai_finops_enforcement_actions` + des Events d'enforcement. Un **finalizer** purge les séries
d'enforcement de la policy à sa suppression. Les findings détaillés apparaissent dans
[AIFinOpsReport](aifinopsreport.md).

### Plan Shadow-AI (eBPF, indépendant de la gateway)
Le reconciler exécute **aussi**, et **inconditionnellement** (même sans AIGateway), une détection
de **shadow-AI** : il lit l'egress par pod observé par eBPF (Tetragon) dans le ConfigMap
conventionnel **`shadow-egress`** du namespace de la policy (clé `egress.json`), classe chaque
destination par zone via le [catalogue](../features/catalog.md) et expose
`ai_finops_shadow_ai_egress{namespace,application,zone,provider,severity}` + des Events `ShadowAI`.
Cela capte les appels LLM qui **contournent** la gateway. Détails :
[shadowengine](../features/shadowengine.md) · [DASHBOARDS](../DASHBOARDS.md).

## Exemple
[`..._aisovereigntypolicy.yaml`](../../config/samples/aiops_v1alpha1_aisovereigntypolicy.yaml).
