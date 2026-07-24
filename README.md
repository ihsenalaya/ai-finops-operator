# AI FinOps Operator

Opérateur Kubernetes **standalone** de gouvernance FinOps et de souveraineté du trafic IA.
Il répond à une question simple que toute équipe qui expose des LLM en production finit par
se poser : *combien ça coûte, à qui l'attribuer, est-ce que ça reste dans les zones de
résidence autorisées, et peut-on changer de modèle sans casser la qualité ni le budget ?*
Il installe **11 CRDs** sous le groupe API `aiops.imperium.io/v1alpha1`, tourne de manière
autonome (aucune dépendance à l'exécution sur un autre opérateur), et reste **read-mostly**
par défaut : il observe la télémétrie réelle d'une gateway IA, calcule des scores et des
coûts auditables, publie des statuts/métriques/rapports — et ne touche au plan de données
que lorsqu'un chemin d'enforcement est explicitement activé.

Public visé : équipes plateforme / FinOps / conformité qui opèrent une ou plusieurs gateways
IA (Envoy AI Gateway, LiteLLM, Kong, Gateway API...) et veulent un contrôle déclaratif,
auditable et versionné par GitOps de ce que ces gateways font réellement.

## Table des matières

- [Vue d'ensemble](#vue-densemble)
- [Fonctionnalités](#fonctionnalités)
- [Architecture](#architecture)
- [CRDs](#crds-11)
  - [AIProvider](#aiprovider) · [AIModel](#aimodel) · [AIGateway](#aigateway) ·
    [AIBudgetPolicy](#aibudgetpolicy) · [AISovereigntyPolicy](#aisovereigntypolicy) ·
    [AIBreakEvenAnalysis](#aibreakevenanalysis) · [AIFinOpsReport](#aifinopsreport) ·
    [AIQualityGate](#aiqualitygate) · [AIRoutingPolicy](#airoutingpolicy) ·
    [AIRouteOverride](#airouteoverride) · [AIChangeRequest](#aichangerequest)
- [Calculs de score, qualité et routage](#calculs-de-score-qualité-et-routage)
  - [1. Score de routage global](#1-score-de-routage-global)
  - [2. Décomposition des coûts](#2-décomposition-des-coûts)
  - [3. Évaluation de souveraineté](#3-évaluation-de-souveraineté)
  - [4. AI Quality Score (composite)](#4-ai-quality-score-composite)
  - [5. Couche statistique optionnelle](#5-couche-statistique-optionnelle-pkgqualitystats)
  - [6. Évaluation budgétaire](#6-évaluation-budgétaire)
  - [7. Analyse de point mort (break-even)](#7-analyse-de-point-mort-break-even)
  - [8. Enforcement et fallback budgétaire](#8-enforcement-et-fallback-budgétaire)
- [Installation](#installation)
- [Démarrage rapide kind (tout-en-un)](#démarrage-rapide-kind-tout-en-un)
- [Console graphique](#console-graphique)
- [Configuration](#configuration)
- [Métriques Prometheus](#métriques-prometheus)
- [Sécurité / bonnes pratiques de déploiement](#sécurité--bonnes-pratiques-de-déploiement)
- [Dépannage](#dépannage)
- [Intégration avec d'autres opérateurs](#intégration-avec-dautres-opérateurs-optionnelle)
- [Contribuer](#contribuer)
- [License](#license)

## Vue d'ensemble

```
ai-finops-operator/
├── README.md                      ← ce fichier
├── LICENSE                        ← Apache-2.0
├── api/v1alpha1/                  ← types des 11 CRDs
├── internal/controller/           ← 11 reconcilers + moteurs d'enforcement/métriques
├── internal/costengine/           ← décomposition des coûts (pure, sans dépendance K8s)
├── internal/routingscore/         ← score de routage runtime
├── internal/qualityengine/        ← AI Quality Score composite
├── internal/sovereigntyengine/    ← findings de souveraineté (zones, données sensibles)
├── internal/shadowengine/         ← détection shadow-AI (egress eBPF/Tetragon)
├── internal/budgetengine/         ← phase de budget + actions recommandées
├── internal/breakevenengine/      ← point mort managé vs auto-hébergé
├── internal/enforcementengine/    ← mode → action (report/warn/block/reroute)
├── pkg/qualitystats/              ← primitives statistiques (non-infériorité, hystérésis...)
├── cmd/                           ← finops-manager, header-proxy, seed-usage
├── charts/ai-finops-operator/     ← Helm chart (11 CRDs + RBAC scopé)
├── docs/                          ← une fiche de référence complète par CRD
└── automatisation/
    ├── up.sh / down.sh            ← cluster kind complet en une commande
    ├── test-apps/                 ← catalogue, usage gateway, policies+rapport,
    │                                 quality gate, egress shadow-AI, gateway d'éval
    └── dashboards/                ← dashboard Grafana "AI FinOps Operator — Overview"
```

Chaque CRD a sa fiche de référence complète dans `docs/` (champs, comportement du
controller, exemple, `kubectl`). Ce README résume l'essentiel ; **la doc `docs/<crd>.md`
correspondante fait foi pour le détail exhaustif des champs**.

## Fonctionnalités

- **Attribution des coûts en EUR** par requête, modèle, application, équipe, namespace et
  zone de résidence — à partir d'une télémétrie mesurée, jamais simulée silencieusement
  (`internal/costengine`).
- **Budgets avec dégradation gracieuse** : seuils d'alerte configurables, phase `Exceeded`,
  et un **fallback managé** qui peut être réellement actué au gateway au lieu d'un blocage
  brutal, sous plusieurs garde-fous (`internal/budgetengine` + `internal/controller/budgetfallback.go`).
- **Souveraineté du trafic IA** : contraintes de résidence des données (zones autorisées /
  interdites), règles données-sensibles, findings par flux (namespace/application/modèle/
  fournisseur), et un mode d'enforcement qui va du simple constat au reroute/blocage réel
  dans Envoy AI Gateway (`internal/sovereigntyengine` + `internal/enforcementengine`).
- **Détection shadow-AI (eBPF/Tetragon)** : indépendamment de toute gateway, l'opérateur lit
  l'egress par pod observé par eBPF (Tetragon) et détecte les appels LLM directs qui
  **contournent** la gateway gouvernée, dans une zone non conforme (`internal/shadowengine`).
- **Break-even managé vs auto-hébergé** : à quel volume un déploiement GPU auto-hébergé
  devient rentable face à l'API managée, avec délai de retour sur investissement
  (`internal/breakevenengine`).
- **Rapports FinOps consolidés** : un `ConfigMap` Markdown + JSON par namespace/période,
  consommable par un humain ou un pipeline (`AIFinOpsReport`).
- **Quality gates avant changement de modèle** : score composite multi-dimensions
  (correction, fiabilité, latence, similarité sémantique, jugement LLM optionnel), avec une
  couche statistique de non-infériorité optionnelle façon essai clinique
  (`internal/qualityengine` + `pkg/qualitystats`).
- **Optimisation de routage continue** : score runtime coût/qualité/latence/fiabilité par
  modèle observé, avec garde-fous (qualité minimale, latence maximale, conformité
  souveraineté) et blocage explicite du candidat qui les enfreint (`AIRoutingPolicy`).
- **Reroute manuel réversible** (`AIRouteOverride`) et **workflow d'approbation humaine**
  pour les changements sensibles (`AIChangeRequest`), y compris l'intégration optionnelle au
  plan GOV-AR (réservation/calibration/drift) d'un opérateur tiers.
- **Observabilité native** : une trentaine de familles de métriques `ai_finops_*` (gauges,
  jamais de `_total` sur un instantané), des Events Kubernetes et des conditions
  `Ready`/`Validated` standard sur chaque CRD.

## Architecture

L'opérateur est **read-mostly** : il observe la télémétrie, calcule, publie des statuts,
des métriques et des rapports — et ne touche au plan de données que lorsqu'un chemin
d'enforcement est explicitement configuré.

```
                        ┌──────────────────────────────────────────────┐
   AIGateway            │              finops-manager                  │
   (source télémétrie)  │                                              │
 prometheus/configmap ─→│ 1. Collecte   usage: requêtes, tokens in/out │
                        │ 2. Valorisation  EUR via tarifs AIProvider/  │
   AIProvider/AIModel ─→│    AIModel (par namespace/app/modèle/zone)   │
   (catalogue+tarifs)   │ 3. Évaluation des politiques                 │
                        │    AIBudgetPolicy   → usage%, phase          │
   AIBudgetPolicy    ─→ │    AISovereigntyPolicy → findings            │
   AISovereigntyPolicy  │    AIBreakEvenAnalysis → économies           │
   AIBreakEvenAnalysis  │    AIQualityGate → score composite           │
                        │    AIRoutingPolicy → recommandations         │
                        │ 4. Publication                               │
                        │    status CRs + events + métriques ai_finops_│
                        │    AIFinOpsReport → ConfigMap (md/json)      │
                        │ 5. Enforcement (opt-in, mode enforce/warn)   │
   Tetragon (eBPF)   ──→│    reroute budget / blocage souveraineté /   │
   (egress shadow-AI)   │    détection shadow-AI                       │
                        └──────────────────────────────────────────────┘
```

1. **Collecte** — `AIGateway` déclare la source de télémétrie (`prometheus`, `configmap`,
   `aigw`, ou `fake` en démo opt-in). Il n'existe **aucun repli silencieux** : sans source
   réelle configurée, l'opérateur pose la condition `NoTelemetrySource` au lieu d'inventer
   des données.
2. **Valorisation** — chaque enregistrement d'usage (application, modèle, tokens
   entrée/sortie) est converti en EUR avec les tarifs du catalogue `AIProvider`/`AIModel`,
   puis ventilé par namespace, application, modèle, équipe et **zone de résidence** (EU/US…).
3. **Évaluation des politiques** — à chaque réconciliation, chaque CRD de politique
   applique son propre moteur pur (voir [Calculs](#calculs-de-score-qualité-et-routage)).
4. **Publication** — statuts typés sur chaque CR (`kubectl get aibudget` montre la phase),
   Events Kubernetes, métriques `ai_finops_*`, et `AIFinOpsReport` qui génère un
   **ConfigMap** avec le rapport complet en Markdown + JSON.
5. **Enforcement (opt-in)** — `enforcementMode: reportOnly → warn → enforce`. En `enforce`
   avec une Envoy AI Gateway configurée, l'opérateur actue réellement : reroute budget vers
   le fallback managé conforme, blocage/reroute souveraineté, reroute manuel immédiat
   (`AIRouteOverride`) et changement gouverné par approbation humaine (`AIChangeRequest`).
6. **Détection shadow-AI** — en parallèle et **inconditionnellement**, `AISovereigntyPolicy`
   lit l'egress observé par eBPF (Tetragon) pour détecter le trafic qui contourne la gateway.

## CRDs (11)

Groupe API : `aiops.imperium.io/v1alpha1`, toutes namespaced. Le manager
(`cmd/finops-manager`) enregistre exactement les 11 controllers — pas de webhook dans ce
dépôt, pas d'écriture hors des chemins d'enforcement déclarés.

Chaque section ci-dessous donne d'abord l'idée en une phrase, puis les champs les plus
utilisés. Pour la liste exhaustive des champs, statuts et exemples : suivre le lien
`docs/`.

### AIProvider
*shortName `aiprov`* — [docs/aiprovider.md](docs/aiprovider.md)

**En clair** : vous déclarez "j'ai un compte Mistral en France, voici son tarif au million
de tokens", et l'opérateur sait désormais valoriser tout usage de ce fournisseur et vérifier
sa zone de résidence.

| Champ | Description |
|---|---|
| `type` | `openai\|azure-openai\|mistral\|anthropic\|bedrock\|vertex\|self-hosted\|custom`. |
| `dataResidency` | Zone de traitement (ex. `france`, `eu`, `us`) — consommée par la souveraineté. |
| `managed` | `true` = API managée, `false` = auto-hébergé GPU. |
| `pricing.inputTokenPricePerMillion` / `outputTokenPricePerMillion` | Prix par million de tokens (obligatoires). |
| `compliance.allowedForSensitiveData` | Autorise ce fournisseur pour de la donnée sensible. |

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIProvider
metadata:
  name: mistral-fr
spec:
  type: mistral
  dataResidency: france
  managed: true
  pricing: { inputTokenPricePerMillion: "2.00", outputTokenPricePerMillion: "6.00" }
  compliance: { allowedForSensitiveData: true }
```

### AIModel
[docs/aimodel.md](docs/aimodel.md)

**En clair** : vous cataloguez "gpt-4o existe, il appartient à ce provider, il est de
qualité `high`", et cette entrée devient la clé utilisée partout ailleurs (coûts, scores,
gates) pour désigner ce modèle.

| Champ | Description |
|---|---|
| `providerRef` * | Nom de l'`AIProvider` (même namespace). |
| `modelName` * | Identifiant côté fournisseur (ex. `gpt-4o`) — clé du price-book. |
| `qualityTier` | `low\|medium\|high` — alimente le `QualityScore` du routage (voir calculs). |
| `sensitiveDataAllowed` | Ce modèle précis est-il cléré pour de la donnée sensible. |

\* requis. Statut : `resolvedProvider`, `lastQualityScore`/`lastEvaluatedAt` (résumé de la
dernière évaluation `AIQualityGate` qui ciblait ce modèle).

### AIGateway
*shortName `aigw`* — [docs/aigateway.md](docs/aigateway.md)

**En clair** : vous pointez l'opérateur vers une gateway IA qui existe déjà (elle n'est
jamais créée ni modifiée par ce CRD) et vous lui dites comment en lire la télémétrie.

| Champ | Description |
|---|---|
| `type` | `litellm\|envoy\|kong\|gateway-api\|custom`. |
| `endpoint` * | URL de base de la gateway. |
| `namespaceSelector` | Namespaces gouvernés ; `nil` = aucun (défaut sûr). |
| `telemetry.mode` | `prometheus\|aigw\|configmap\|fake` — `fake` est un opt-in de démo, jamais un repli silencieux. |

### AIBudgetPolicy
*shortName `aibudget`* — [docs/aibudgetpolicy.md](docs/aibudgetpolicy.md)

**En clair** : "l'équipe RH a 500 EUR/mois pour l'IA ; au-delà de 90 % avertis-moi, et à
100 % bascule automatiquement sur un modèle de secours moins cher — si je l'active."

| Champ | Description |
|---|---|
| `target.namespace/team/application` | Portée du budget (filtres cumulatifs). |
| `budgetEUR` * | Plafond sur la période. |
| `period` | `daily\|weekly\|monthly` (défaut `monthly`). |
| `warningThresholdPercent`/`criticalThresholdPercent`/`hardLimitPercent` | Seuils (défauts 70/90/100). |
| `fallbackModelRef` | `AIModel` managé cible pour le fallback live. |
| `enforcementMode` | `reportOnly\|warn\|enforce` — `enforce` seul autorise l'actuation réelle. |
| `fallbackOnPhase` | Phase minimale qui peut déclencher le fallback (défaut `Exceeded`). |
| `minFallbackQualityTier` / `maxFallbackLatencyMillis` / `maxFallbackErrorPercent` | Garde-fous du fallback. |

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIBudgetPolicy
metadata:
  name: hr-budget
spec:
  target: { team: hr }
  budgetEUR: "500.00"
  period: monthly
  fallbackModelRef: { name: mistral-small }
  enforcementMode: enforce
  fallbackOnPhase: Exceeded
```

### AISovereigntyPolicy
*shortName `aisov`* — [docs/aisovereigntypolicy.md](docs/aisovereigntypolicy.md)

**En clair** : "seuls FR/EU sont autorisés, les US sont interdits ; si un flux atterrit
quand même chez un fournisseur américain, avertis-moi (ou bloque/reroute-le si je passe en
`enforce`)." Détecte aussi le trafic qui contourne complètement la gateway.

| Champ | Description |
|---|---|
| `dataResidency.allowedZones[]` / `forbiddenZones[]` | Zones autorisées / interdites (ex. `FR`, `EU`, `US`). |
| `sensitiveData.externalProvidersAllowed` | Autorise les fournisseurs externes pour la donnée sensible. |
| `sensitiveData.requireAnonymization` | Exige l'anonymisation (rappel informatif, non appliqué par l'opérateur lui-même). |
| `enforcementMode` | `reportOnly\|warn\|enforce`. |

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AISovereigntyPolicy
metadata:
  name: eu-only
spec:
  dataResidency: { allowedZones: [FR, EU], forbiddenZones: [US, CN] }
  sensitiveData: { externalProvidersAllowed: false }
  enforcementMode: warn
```

### AIBreakEvenAnalysis
*shortName `aibreakeven`* — [docs/aibreakevenanalysis.md](docs/aibreakevenanalysis.md)

**En clair** : "je paie l'API managée pour ce modèle ; si je louais un GPU pour le
remplacer, est-ce rentable et en combien de mois je rembourse la migration ?"

| Champ | Description |
|---|---|
| `currentModelRef` * | `AIModel` actuellement utilisé (API managée). |
| `alternativeSelfHosted.monthlyGpuCostEUR` * | Coût GPU mensuel de l'alternative. |
| `alternativeSelfHosted.estimatedOpsCostEUR` / `storageNetworkCostEUR` | Coûts ops et stockage/réseau mensuels. |
| `alternativeSelfHosted.migrationCostEUR` | Coût one-shot de migration. |
| `analysisWindowDays` | Fenêtre d'observation extrapolée au mois (défaut 30). |

### AIFinOpsReport
*shortName `aireport`* — [docs/aifinopsreport.md](docs/aifinopsreport.md)

**En clair** : "génère-moi, tous les mois, un rapport lisible par un humain (et par une
machine) avec le coût total, le top des modèles, les problèmes de souveraineté et les
recommandations d'économies."

| Champ | Description |
|---|---|
| `target.namespace` | Namespace ciblé (filtre l'usage). |
| `target.period` | `daily\|weekly\|monthly` (défaut `monthly`). |
| `gatewayRef` | `AIGateway` dont on prend le mode de télémétrie. |

Le rapport complet est écrit dans un **ConfigMap** `<nom>-report` (clés `report.md`,
`report.json`), en owner-reference du CR (garbage-collecté avec lui). Re-réconcilie toutes
les 60 s.

### AIQualityGate
*shortName `aiqgate`* — [docs/aiqualitygate.md](docs/aiqualitygate.md)

**En clair** : "avant de basculer le chatbot RH de gpt-4o vers mistral-large, prouve-moi
avec des vrais prompts et de la vraie télémétrie que la qualité ne se dégrade pas au-delà
de ma tolérance." Un gate par application est le grain prévu.

| Champ | Description |
|---|---|
| `target.namespace/application` * | Application protégée. |
| `sourceModel` / `candidateModel` * | Modèle actuel / candidat. |
| `goldenDatasetRef` * | ConfigMap de prompts métier de référence. |
| `evidenceRef` | ConfigMap des sorties déterministes d'un run golden (preuve auditable du score). |
| `weights.correctness/reliability/latency/semantic/judged` | Poids du score composite (défaut 0.40/0.20/0.15/0.15/0.10). |
| `latencyThresholdMs` | Seuil de la dimension `Latency`. |
| `tolerancePoints` | Tolérance de dégradation candidat vs source (défaut 3 points). |
| `judge.enabled` | Active la dimension `Judged` (juge LLM souverain). |
| `statistical` | Active la couche statistique de non-infériorité + hystérésis (voir [calculs](#5-couche-statistique-optionnelle-pkgqualitystats)). |
| `canary.enabled/percent/duration` | Contrat de canary attendu avant reroute complet. |

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIQualityGate
metadata:
  name: hr-chatbot-migration
spec:
  target: { namespace: hr, application: chatbot }
  sourceModel: gpt-4o
  candidateModel: mistral-large
  goldenDatasetRef: { name: hr-chatbot-prompts }
  tolerancePoints: 3
```

### AIRoutingPolicy
*shortName `airpolicy`* — [docs/airoutingpolicy.md](docs/airoutingpolicy.md)

**En clair** : "parmi tous les modèles réellement utilisés par cette application, dis-moi
en continu lequel serait le meilleur choix coût/qualité/latence, et bloque la
recommandation si elle enfreint mes garde-fous."

| Champ | Description |
|---|---|
| `objective` | `cost\|quality\|latency` — dimension d'optimisation déclarée. |
| `guardrails.minQualityScore` | Score de routage minimal du candidat (défaut 0.70). |
| `guardrails.maxLatencyMillis` | Latence moyenne mesurée maximale acceptée. |
| `guardrails.requireSovereigntyCompliance` | Restreint aux candidats conformes souveraineté. |
| `canary.enabled/percent` | Validation canary attendue avant reroute complet. |
| `govar` | Politique GOV-AR typée (réservation/calibration/drift), optionnelle, pour l'opérateur GOV-AR tiers. |

Le statut expose `recommendations[]` (`currentModel`, `candidateModel`, `candidateScore`,
`blocked`/`blockReason`) — voir les nuances d'implémentation dans
[Calculs](#1-score-de-routage-global).

### AIRouteOverride
*shortName `airoverride`* — [docs/airouteoverride.md](docs/airouteoverride.md)

**En clair** : "bascule immédiatement tout le trafic de gpt-4o vers mistral-large, tout de
suite, sans attendre le moteur d'optimisation." Supprimer l'objet restaure la route
d'origine.

| Champ | Description |
|---|---|
| `sourceModel` * / `targetModel` * | Modèle qui sert le trafic / modèle cible. |
| `reason` | Justification lisible (audit). |

Statut : `phase` (`Pending → Actuated \| Failed \| Reverted`), `actuatedRoutes[]`.
Nécessite une Envoy AI Gateway accessible ; sinon reste `Pending` avec condition explicative.

### AIChangeRequest
*shortName `aicrq`* — [docs/aichangerequest.md](docs/aichangerequest.md)

**En clair** : "je propose de basculer vers ce modèle, avec l'économie attendue et le
niveau de risque ; un humain doit approuver avant que ça devienne réel."

| Champ | Description |
|---|---|
| `action` * | `reroute` (changement de routage) ou `authorize-gov-ar-route` (route GOV-AR exacte, opérateur tiers). |
| `approval` | `Pending\|Approved\|Rejected` — décision du reviewer humain. |
| `sourceModel`/`targetModel`, `expectedSavingEUR`, `qualityScore`, `riskLevel`, `reason` | Dossier de décision présenté au reviewer. |
| `expiresAfter` | Délai avant expiration automatique. |
| `requestedBy` | Identité authentifiée du demandeur — tamponnée par un webhook tiers (GOV-AR), non déclarative. |

Statut : `phase` (`Pending → Approved → Actuated \| Rejected \| Expired \| Failed`).
`AIRoutingPolicy` ne crée pas ces objets automatiquement aujourd'hui : ses
`recommendations[]` sont une entrée pour qu'un humain ou un pipeline crée l'`AIChangeRequest`
correspondant.

## Calculs de score, qualité et routage

Cette section documente **exactement** les formules implémentées, avec le fichier source
qui fait foi. Les moteurs sont tous **purs** (aucune dépendance Kubernetes), donc
testables unitairement et lisibles indépendamment des controllers qui les appellent.

### 1. Score de routage global

Fichier : [`internal/routingscore/score.go`](internal/routingscore/score.go). Calculé pour
chaque tuple (namespace, application, modèle, provider) observé avec une vraie télémétrie.

```
Score = 0.40 × CostScore + 0.30 × QualityScore + 0.20 × LatencyScore + 0.10 × ReliabilityScore
```

Poids par défaut (`DefaultWeights()`) : Coût 0.40, Qualité 0.30, Latence 0.20,
Fiabilité 0.10 — surchargeables programmatiquement, doivent sommer à 1.

- **La souveraineté est une porte, pas une composante pondérée.** Si le fournisseur du
  modèle est dans une zone interdite par l'`AISovereigntyPolicy` active
  (`SovereigntyCompliant == false`), le `Score` entier est forcé à **0**, quels que soient
  les autres composants — ce modèle n'est jamais recommandé. Si aucune politique de
  souveraineté ne s'applique (`SovereigntyChecked == false`), le modèle est considéré
  conforme par défaut.
- **CostScore** : normalisation min-max inversée du coût par requête, calculée sur tous les
  modèles observés **dans le même lot** :
  `CostScore = 1 - (CostPerRequest - min) / (max - min)`, bornée à [0,1]. Le modèle le
  moins cher observé obtient 1, le plus cher obtient 0. Si tous les modèles ont le même
  coût, tous obtiennent 1 (pas de division par zéro : le code renvoie 1 quand `max <= min`).
- **QualityScore (routage uniquement)** — **attention, ce n'est PAS l'AI Quality Score
  composite** décrit en section 4, mais une simple correspondance catalogue :
  `high → 1.0`, `medium → 0.75`, `low → 0.50`, autre/non renseigné → `0.60`. Les deux ne
  doivent jamais être confondus.
- **LatencyScore** : même normalisation min-max inversée que le coût, sur la latence
  **observée** (moyenne pondérée par le nombre de requêtes par modèle). Si un modèle n'a
  aucune télémétrie de latence (`LatencyTelemetryAvailable == false`), son `LatencyScore`
  est forcé à la valeur **neutre 0.5** (constante `NeutralLatencyScore`) et `LatencySource`
  vaut `"unavailable"` au lieu de `"observed"` — le moteur ne fabrique jamais un chiffre de
  latence.
- **ReliabilityScore** : `1 - errors/requests`, borné à [0,1] ; un modèle sans aucune
  requête obtient 1 (aucune preuve d'échec).

**Métriques Prometheus émises** (voir la [section métriques](#métriques-prometheus) pour
le détail complet) : `ai_finops_routing_score`, `ai_finops_cost_score`,
`ai_finops_latency_score`, `ai_finops_reliability_score`, `ai_finops_sovereignty_score`.
**Point d'attention important** : la métrique nommée `ai_finops_quality_score` **n'est pas**
la `QualityScore` (tier catalogue) de ce moteur de routage — elle est exclusivement émise
par le controller `AIQualityGate` et porte l'**AI Quality Score composite** (section 4,
échelle 0–100, par dimension). La `QualityScore` interne au routage (échelle 0–1, simple
lookup de tier) n'a pas de métrique dédiée à son nom ; elle n'existe qu'agrégée dans
`ai_finops_routing_score`. Voir `internal/controller/qualitygatemetrics.go` (seul writer de
`ai_finops_quality_score`) et `internal/controller/reportmetrics.go` /
`internal/controller/aifinopsreport_controller.go` (writers de `ai_finops_routing_score`,
`ai_finops_cost_score`, etc.) pour la vérification exacte des propriétaires de métriques.

**Nuance d'implémentation (`AIRoutingPolicy`)** : le champ `spec.objective`
(`cost|quality|latency`) est actuellement **déclaratif** — le controller
(`internal/controller/airoutingpolicy_controller.go`) calcule toujours le score avec
`routingscore.DefaultWeights()` et ne repondère pas selon `objective`. Le garde-fou
`guardrails.minQualityScore` compare en réalité au **score global** `best.Score` (pas à la
seule composante `QualityScore`), malgré son nom. Le champ de statut
`recommendations[].estimatedSavingsEUR` est déclaré dans le type mais n'est pas encore
renseigné par ce controller (le calcul d'économies chiffrées existe côté
`AIFinOpsReport`/`ai_finops_cost_saving_eur`, pas ici). Ce sont des limitations documentées,
pas des bugs cachés.

### 2. Décomposition des coûts

Fichier : [`internal/costengine/costengine.go`](internal/costengine/costengine.go). Pure
comptabilité de dépense, **pas un score** :

```
cost = inputTokens/1 000 000 × inputPricePerMillion + outputTokens/1 000 000 × outputPricePerMillion
```

Sommé et ventilé `ByModel` / `ByProvider` / `ByNamespace` / `ByTeam` / `ByApplication`. Les
modèles vus dans la télémétrie mais absents du catalogue de prix sont suivis dans
`UnpricedModels` (comptés à coût zéro, **jamais silencieusement ignorés** — surfacés
explicitement). Helpers `AvgCostPerRequest` / `CostPerToken`. La devise est prise du premier
modèle tarifé rencontré ; le MVP **ne convertit pas** les devises mixtes (limitation
documentée, pas un bug).

### 3. Évaluation de souveraineté

Fichier : [`internal/sovereigntyengine/sovereigntyengine.go`](internal/sovereigntyengine/sovereigntyengine.go).
Ce n'est **pas** un score numérique dans ce package — il émet des **Findings** (sévérité
`critical`/`warning`/`info`) que le score de routage transforme en porte à 0 (section 1) :

- Un fournisseur dont la zone normalisée est dans `forbiddenZones` → finding **critical**,
  blocage dur.
- Un fournisseur dont la zone n'est couverte par aucune `allowedZones` (liste vide =
  aucune restriction positive, seul `forbiddenZones` s'applique ; `"EU"` en zone autorisée
  couvre 18 codes pays membres de l'UE) → finding **warning**.
- Un fournisseur managé externe utilisé alors que `externalProvidersAllowed: false` →
  finding **warning** (règle données-sensibles) ; l'évaluation par flux (`EvaluateFlows`)
  signale en plus un flux si le fournisseur **ou** le modèle précis n'est pas cléré pour de
  la donnée sensible.
- `requireAnonymization: true` → finding **info**, un simple rappel que la gateway doit
  rédiger les champs sensibles — **l'opérateur ne le fait pas lui-même**.
- `EvaluateFlows` déduplique les findings identiques sur de nombreux échantillons et
  additionne les volumes de requêtes affectés, donc la sortie est **une ligne par
  violation distincte**, pas une par requête.

### 4. AI Quality Score (composite)

Fichier : [`internal/qualityengine/qualityengine.go`](internal/qualityengine/qualityengine.go),
utilisé par `AIQualityGate` pour comparer un modèle candidat au modèle source actuel.

```
Overall = 0.40×Correctness + 0.20×Reliability + 0.15×Latency + 0.15×Semantic + 0.10×Judged
```

Poids par défaut (`DefaultWeights()`) ; renormalisés à somme 1 si personnalisés. Si le juge
LLM est désactivé, le poids `Judged` est mis à zéro **avant** renormalisation — les 4 autres
dimensions absorbent automatiquement sa part. Les 5 dimensions sont sur une échelle 0–100.

- **Correctness** : par échantillon d'evidence golden, moyenne jusqu'à 4 vérifications
  applicables :
  - (a) `ReferenceCorrectnessScore` quand une paire attendu/obtenu existe : 100 si match
    exact après normalisation du texte, sinon
    `0.60×ContentTokenCoverage + 0.25×TokenF1 + 0.15×RougeL` ;
  - (b) mots-clés requis présents, en 100/0 ;
  - (c) validité JSON en 100/0, quand le champ est censé être (ou ressemble à) du JSON ;
  - (d) F1 au niveau champ quand des champs structurés attendu/obtenu sont fournis ;
  - puis moyenne sur l'ensemble des échantillons.
- **Reliability** : `100 × (1 - (errors+timeouts+invalidJSON)/requests)`, à partir de vraie
  télémétrie.
- **Latency** : linéaire par morceaux contre un seuil configuré — `≤ seuil/2` → 100 (note
  pleine) ; `≥ 2×seuil` → 0 ; entre `seuil/2` et `seuil` → linéaire 100→50 ; entre `seuil`
  et `2×seuil` → linéaire 50→0.
- **Semantic** : moyenne de scores de similarité sémantique **injectés de l'extérieur**
  (le moteur ne calcule aucun embedding lui-même ; l'appelant fournit `SemanticScore` par
  échantillon) — documenté comme "branchable", pas comme du NLP natif.
- **Judged** : moyenne de scores de jugement LLM **injectés de l'extérieur** (même
  principe branchable) ; poids nul et ignoré quand le juge est désactivé.
- **Règle anti-fabrication** : si une dimension à poids non nul n'a aucun signal
  exploitable, le résultat entier est verdict `insufficient-data` — **jamais un chiffre
  deviné**.
- **Verdict** (comparaison candidat vs source) : `candidate-safe` si
  `Candidate.Overall >= Source.Overall - tolerancePoints` (tolérance par défaut 3 points sur
  100), sinon `candidate-risk`.

### 5. Couche statistique optionnelle (`pkg/qualitystats`)

Fichier : [`pkg/qualitystats/qualitystats.go`](pkg/qualitystats/qualitystats.go). Une couche
statistique plus rigoureuse, **optionnelle**, disponible pour les décisions de
canary/qualité. **Elle est réellement branchée sur une CRD** : `AIQualityGate.spec.statistical`
(type `AIQualityStatisticalSpec` dans `api/v1alpha1/aiqualitygate_types.go`) l'active et la
configure — ce n'est pas une bibliothèque orpheline. Voir le câblage exact dans
`internal/controller/aiqualitygate_controller.go` (autour de `statisticalModeEnabled`,
`qualitystats.EvaluateNonInferiority`, `qualitystats.EvaluateComposite`,
`qualitystats.ApplyHysteresis`).

- **Test de non-infériorité** (façon essai clinique) : calcule la taille d'échantillon
  requise par bras
  `n = 2×(zα+zβ)²×p×(1-p)/Δ²` (défauts : `Δ=0.05`, confiance 95 %, puissance 80 %, taux de
  succès de référence 90 % — configurables via `nonInferiorityDelta`, `confidenceLevel`,
  `power`, `baselineSuccessRate`), puis déclare `candidate-safe` seulement si la borne
  basse de confiance de `(tauxCandidat - tauxSource)` est `≥ -Δ`, sinon `candidate-risk`,
  ou `insufficient-data` si l'un des deux bras n'a pas encore atteint la taille
  d'échantillon requise.
- **Score composite** (un mélange à 4 poids **distinct** de l'AI Quality Score ci-dessus) :
  `Score = 0.40×Quality + 0.20×ErrorRate + 0.20×LatencyP95 + 0.20×Cost`
  (`DefaultCompositeWeights()`, configurable via `spec.statistical.compositeWeights`),
  chaque entrée pré-bornée à [0,1]. Dans le controller, `Quality` = `Candidate.Overall/100`,
  `ErrorRate` et `LatencyP95` viennent des **scores de régression par paires** ci-dessous, et
  `Cost` d'une comparaison de coût source/candidat.
- **Hystérésis** anti-flapping entre les verdicts safe/risk : n'**entre** en "safe"
  qu'au-dessus de 0.80, n'en **sort** qu'en dessous de 0.70
  (`hysteresisEnterScore`/`hysteresisExitScore`) — un score entre 0.70 et 0.80 conserve le
  verdict précédent.
- **Scores de régression par paires** (`PercentileScore`/`ErrorRateScore`/`CostScore` — le
  `CostScore` de **ce** package, distinct de `internal/costengine`) : notent un candidat
  contre une baseline source sous forme de ratio. Par exemple `PercentileScore` donne 1 si
  candidat ≤ source, 0 si candidat ≥ source × `maxRegressionRatio` (défaut 1.25, codé en dur
  dans l'appel du controller — pas exposé comme champ de CRD aujourd'hui), linéaire entre
  les deux. Ces trois fonctions sont utilisées **automatiquement** par le controller quand
  le mode statistique est actif ; elles ne sont pas configurables individuellement depuis le
  spec au-delà de `maxLatencyIncreasePercent`/`maxErrorRatePercent` déjà présents dans
  `requiredChecks`.

En résumé côté CRD : `AIQualityGate.spec.statistical.{nonInferiorityDelta, confidenceLevel,
power, baselineSuccessRate, hysteresisEnterScore, hysteresisExitScore, compositeWeights}`
sont les champs réellement exposés ; le reste (scores de régression par paires, taille
d'échantillon) est calculé automatiquement par le controller à partir de ces réglages.

### 6. Évaluation budgétaire

Fichier : [`internal/budgetengine/budgetengine.go`](internal/budgetengine/budgetengine.go).

```
usagePercent = round(spend / budget × 100)
```

Échelle de phases : `WithinBudget` → `Warning` (≥ `warningThresholdPercent`) → `Critical`
(≥ `criticalThresholdPercent`) → `Exceeded` (≥ `hardLimitPercent`), chaque palier
déclenchant sa propre liste d'actions recommandées configurée (**report-only dans le MVP** —
ce moteur lui-même n'enforce rien ; le chemin d'actuation réel est décrit en section 8).
`budget <= 0` → phase `Unknown`, message explicite "cannot evaluate", **jamais un 0 %
silencieux**.

### 7. Analyse de point mort (break-even)

Fichier : [`internal/breakevenengine/breakevenengine.go`](internal/breakevenengine/breakevenengine.go).

```
managedMonthly    = managedTokenCostMonthly + providerFixedMonthly
selfHostedMonthly = gpuMonthly + opsMonthly + storageNetworkMonthly
monthlySavings    = managedMonthly - selfHostedMonthly
```

Si `savings <= 0` → recommandation `keep-managed`. Sinon
`paybackMonths = round(migrationCost / savings, 1 décimale)` (0 si aucun coût de migration
fourni) et la recommandation est `self-host` si `paybackMonths <= paybackThreshold` (défaut
6 mois), sinon `investigate`. `ExtrapolateMonthly` ramène un coût observé sur N jours à un
mois de 30 jours par simple règle de trois.

### 8. Enforcement et fallback budgétaire

Deux fichiers qui transforment un constat en action **réellement actuée** au gateway.

**`internal/enforcementengine/enforcementengine.go`** — le point unique qui décide *quoi*
faire d'une violation de souveraineté, selon le mode :

| Mode | Action | `Actuated` |
|---|---|---|
| `reportOnly` (défaut) | `report` | `true` immédiatement (constat seul, jamais de blocage). |
| `warn` | `warn` | `true` immédiatement (Event + métrique, ne bloque pas). |
| `enforce` | `reroute` si un modèle de fallback conforme est fourni, sinon `block` | `false` tant que le controller n'a pas réellement muté la route au gateway — le moteur pur ne se ment jamais à lui-même sur ce qui s'est passé. |

**`internal/controller/budgetfallback.go`** — décide si/comment router le trafic budgétaire
vers `fallbackModelRef`. Le fallback n'est envisagé que si **toutes** ces conditions sont
vraies :
- `fallbackModelRef` est renseigné et `enforcementMode == enforce` (sinon le fallback reste
  "advisory", jamais actué) ;
- la phase de budget courante a atteint `fallbackOnPhase` (défaut `Exceeded`) ;
- **aucune** `AISovereigntyPolicy` n'est déjà en `enforce` (évite un conflit entre deux
  moteurs qui rerouteraient la même route) ;
- le modèle de fallback résout vers un `AIProvider.managed=true`, respecte
  `minFallbackQualityTier` si défini, et respecte `maxFallbackLatencyMillis` /
  `maxFallbackErrorPercent` si la télémétrie du collector configuré les fournit (sinon le
  fallback est refusé plutôt que d'ignorer le garde-fou) ;
- le fallback est **effectivement moins cher** que le modèle courant sur le mix de tokens
  réellement observé (comparaison de coût réel, pas seulement de tier) ;
- le modèle candidat au reroute n'est **pas partagé** par un usage en dehors de la cible du
  budget (pour ne pas rerouter le trafic d'une autre équipe qui utilise le même modèle).

Le reroute est réversible ; sa cause exacte (y compris pourquoi il n'a *pas* eu lieu)
apparaît dans `status.fallbackReason`.

## Installation

### Prérequis

- Kubernetes ≥ 1.29, Helm ≥ 3.12.
- *Optionnel* : Prometheus Operator (ServiceMonitor), Grafana, une gateway IA
  (Envoy AI Gateway, LiteLLM…) comme plan de données pour l'enforcement réel, Tetragon pour
  la détection shadow-AI.

### Depuis le registre d'images publié

```bash
helm install finops ./charts/ai-finops-operator \
  --namespace finops-system --create-namespace
```

L'image par défaut est `ghcr.io/ihsenalaya/ai-finops-operator/finops-operator`
(tag = `appVersion` du chart, actuellement `0.1.0`). Valeurs utiles :

```bash
# Image locale (kind) :
--set image.repository=finops-operator --set image.tag=dev --set image.pullPolicy=Never
# Prometheus Operator :
--set metrics.serviceMonitor.enabled=true --set metrics.serviceMonitor.labels.release=monitoring
# HA :
--set replicaCount=2 --set leaderElection.enabled=true
```

### Vérification

```bash
kubectl -n finops-system get deploy
kubectl get crd | grep aiops.imperium.io   # 11 CRDs FinOps
```

## Démarrage rapide kind (tout-en-un)

```bash
cd automatisation
./up.sh          # kind + Prometheus/Grafana + opérateur + apps de test + dashboard
```

Le script :

1. crée le cluster kind `finops-operator` ;
2. construit l'image depuis le `Dockerfile` à la racine du dépôt et la charge dans kind ;
3. installe `kube-prometheus-stack` (Grafana admin/admin) ;
4. installe le chart avec ServiceMonitor activé ;
5. déploie les **applications de test** dans `finops-demo` : catalogue 2 providers
   (Mistral EU / OpenAI US) + 3 modèles, gateway en télémétrie `configmap` avec usage mesuré
   statique (chatbot-rh, marketing-assistant, support-triage), budget, politique de
   souveraineté FR/EU (le trafic US déclenche des constats), break-even H100, rapport
   consolidé, un **quality gate** évalué par un vrai job contre un endpoint local
   (`06-eval-gateway`), et un jeu d'**egress observé** (`05-shadow-egress`) qui alimente la
   détection shadow-AI ;
6. importe le dashboard Grafana.

Avec ces applications, **les 22 panels du dashboard sont alimentés**.

Vérifier les résultats :

```bash
kubectl -n finops-demo get aiprov,aimodel,aigw,aibudget,aisov,aireport
kubectl -n finops-demo get aireport monthly-demo-report -o yaml
kubectl -n finops-demo get configmap monthly-demo-report-report -o jsonpath='{.data.report\.md}'
```

Grafana : `kubectl -n monitoring port-forward svc/monitoring-grafana 3000:80`
→ http://localhost:3000 → dashboard **AI FinOps Operator — Overview**
(coûts, budgets, souveraineté, enforcement, scores de routage, radar qualité/coût,
shadow-AI, économies potentielles, quality gates).

Démontage : `./down.sh`.

## Console graphique

Pour créer/modifier les CRDs sans écrire de YAML à la main : une petite interface web
(`ui/console`) et une API REST générique (`console-api`) qui la sert. Aucun compte de
service dédié — l'outil se connecte avec **ton propre kubeconfig** (celui déjà utilisé par
`kubectl`), donc chaque action passe par tes propres droits RBAC sur le cluster.

```bash
# 1. Construire l'interface une fois (ou après une mise à jour)
cd ui/console
npm install
npm run build
cd ../..

# 2. Lancer la console (utilise le contexte kubectl courant)
go run ./cmd/console-api
# → http://localhost:8090
```

Options utiles : `--kubeconfig=/chemin/vers/config`, `--context=mon-cluster`,
`--addr=:9090`. Pendant le développement du frontend, `cd ui/console && npm run dev`
lance un serveur Vite sur `:5173` qui proxifie `/api` vers `console-api` (démarré avec
`-dev-cors`) pour l'itération à chaud.

Ce que fait la console, concrètement : elle lit le schéma OpenAPI de chaque CRD directement
depuis le cluster (`kubectl get crd <nom> -o yaml` sous le capot) et génère un formulaire à
partir de ce schéma — un bouton **« Voir en YAML »** reste disponible à tout moment pour les
utilisateurs qui veulent copier/vérifier le YAML équivalent. La liste des CRDs gérées vient de
`internal/console/registry.go`.

## Configuration

### Flags du manager (`cmd/finops-manager/main.go`)

| Flag | Défaut | Rôle |
|---|---|---|
| `--metrics-bind-address` | `:8080` | Adresse d'écoute `/metrics`. |
| `--health-probe-bind-address` | `:8081` | Adresse des probes `/healthz`/`/readyz`. |
| `--leader-elect` | `false` | Élection de leader (HA, plusieurs replicas). |
| `--metrics-secure` | `false` | Sert `/metrics` en HTTPS si activé. |
| `--enable-http2` | `false` | HTTP/2 désactivé par défaut (mitigation CVE Rapid Reset). |

Ces flags sont exposés dans le chart via `extraArgs` (liste brute ajoutée à la commande du
manager) et les raccourcis `metrics.bindAddress`/`health.bindAddress`/
`leaderElection.enabled` de `values.yaml`.

### Sous-commande `quality-eval`

Le même binaire, invoqué avec `finops-manager quality-eval`, est l'image utilisée par le
`Job` Kubernetes que `AIQualityGate` lance pour interroger un endpoint OpenAI-compatible de
production et produire l'evidence golden (voir `spec.evaluation` du CRD). Flags :
`--endpoint`, `--prompts-dir`/`--prompts-file`, `--namespace`, `--application`,
`--source-model`, `--candidate-model`, `--max-tokens` (défaut 96),
`--timeout-seconds` (défaut 60), `--termination-log` (défaut `/dev/termination-log`).
Vous n'invoquez normalement jamais cette sous-commande vous-même : c'est
`spec.evaluation.image` (par défaut l'image du pod opérateur en cours) qui l'exécute.

### Sources de télémétrie (`AIGateway.spec.telemetry.mode`)

`prometheus` et `aigw` lisent un vrai endpoint de métriques ; `configmap` lit une
`ConfigMap` `usage.json` gérée par vous (utile en démo/CI reproductible) ; `fake` est un
opt-in de démonstration explicite. Il n'y a **aucun mode de repli implicite** — sans mode
configuré ou sans données réelles, les controllers posent `NoTelemetrySource`.

## Métriques Prometheus

Toutes les métriques sont des **gauges** (instantané du dernier reconcile, jamais un
compteur monotone — d'où l'absence de suffixe `_total` sur ces séries, à la différence des
compteurs cumulatifs côté exporter de la gateway). Source de vérité :
`internal/metrics/metrics.go`.

| Métrique | Labels | Rôle |
|---|---|---|
| `ai_finops_requests` | `namespace` | Volume de requêtes attribué. |
| `ai_finops_input_tokens` / `ai_finops_output_tokens` | `namespace` | Volume de tokens. |
| `ai_finops_cost_eur` | `namespace` | Dépense EUR observée. |
| `ai_finops_cost_by_zone_eur` | `zone` | Dépense EUR par zone de souveraineté. |
| `ai_finops_potential_savings_eur` | — | Économie totale estimée des recommandations. |
| `ai_finops_potential_savings_by_app_eur` | `namespace,application` | Économie estimée par workload. |
| `ai_finops_cost_saving_eur` | `namespace,application,current_model,recommended_model` | Économie chiffrée d'un swap concret. |
| `ai_finops_measured_latency_millis` | `namespace,application,model,source` | Latence gateway moyenne observée. |
| `ai_finops_latency_score` | `namespace,application,model,telemetry_available` | Composante latence du score de routage. |
| `ai_finops_routing_score` | `namespace,application,model,latency_telemetry` | Score de routage final (section 1). |
| `ai_finops_cost_score` | `namespace,application,model` | Composante coût du score de routage. |
| `ai_finops_quality_score` | `namespace,app,provider,model,dimension` | **AI Quality Score composite** (section 4) — écrit uniquement par `AIQualityGate`, **pas** la `QualityScore` de routage. |
| `ai_finops_reliability_score` | `namespace,application,model` | Composante fiabilité du score de routage. |
| `ai_finops_sovereignty_score` | `namespace,application,model` | 1 = conforme, 0 = bloqué. |
| `ai_finops_latency_telemetry_available` | `namespace,application,model,source` | 1 si la latence était réellement mesurée. |
| `ai_finops_quality_gate_passed` | `namespace,quality_gate,target_namespace,application,source_model,candidate_model` | 1/0 passage du gate. |
| `ai_finops_quality_gate_failed_checks` | `namespace,quality_gate,target_namespace,application` | Nombre de checks échoués. |
| `ai_quality_gate_score` *(note : sans préfixe `ai_finops_`)* | `namespace,quality_gate,target_namespace,application` | Score composite audité en [0,1] du gate. |
| `ai_finops_projected_monthly_cost_eur` | `namespace` | Projection run-rate à 30 jours. |
| `ai_finops_budget_usage_percent` | `namespace,policy` | Pourcentage de budget consommé. |
| `ai_finops_sovereignty_findings` | `namespace,application,policy,severity` | Nombre de findings. |
| `ai_finops_sovereignty_requests` | `namespace,application,policy,severity` | Requêtes affectées par les findings. |
| `ai_finops_breakeven_savings_eur` | `namespace,analysis` | Économie mensuelle estimée du self-host. |
| `ai_finops_recommendations` | `type,namespace,application,severity` | Recommandations émises. |
| `ai_finops_enforcement_actions` | `policy,namespace,application,mode,action,actuated` | Décision d'enforcement prise (section 8). |
| `ai_finops_shadow_ai_egress` | `namespace,application,zone,provider,severity` | Connexions shadow-AI observées (eBPF/Tetragon). |

Vérification manuelle :

```bash
kubectl -n finops-system port-forward svc/finops-ai-finops-operator-metrics 8080:8080
curl -s localhost:8080/metrics | grep ai_finops_
```

## Sécurité / bonnes pratiques de déploiement

- **RBAC scopé** (`charts/ai-finops-operator/templates/rbac.yaml`) : un unique
  `ClusterRole` (les CRD sont cluster-wide) limité aux 11 types `aiops.imperium.io` (+
  `status`/`finalizers`), aux `ConfigMaps`/`Events`/`Jobs` (nécessaires au reporting et aux
  évaluations `AIQualityGate`), à la lecture `namespaces`/`pods`/`serviceaccounts`, et à la
  mutation des `aigatewayroutes`/`aigatewayroutelists` (`aigateway.envoyproxy.io`)
  **uniquement pour l'enforcement**. Pas d'accès `secrets` en écriture, pas d'accès aux
  workloads applicatifs.
- **Secrets** : `AIGateway.spec.auth.secretRef` référence un `Secret` existant dans le même
  namespace pour un éventuel token admin de gateway ; l'opérateur ne crée ni ne journalise
  jamais de secret en clair.
- **Pod hardening** par défaut dans le chart : `runAsNonRoot`, `seccompProfile:
  RuntimeDefault`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`,
  toutes les `capabilities` droppées ; image distroless `nonroot` (voir `Dockerfile`).
  HTTP/2 désactivé par défaut sur les serveurs metrics/webhook (mitigation CVE Rapid Reset).
- **Enforcement opt-in par conception** : `enforcementMode` par défaut à `reportOnly` sur
  toutes les politiques — un déploiement neuf n'actue jamais rien tant que vous ne l'avez
  pas explicitement activé.
- **Pas de webhook d'admission dans ce dépôt** : la validation avancée fail-closed du
  workflow d'approbation (`AIChangeRequest.requestedBy`/`govarDecision`) provient d'un
  opérateur GOV-AR tiers optionnel ; sans lui, le workflow `reroute` reste utilisable mais
  sans tamponnage d'identité fail-closed.

## Dépannage

| Symptôme | Cause probable | Vérifier |
|---|---|---|
| `Ready=False`, `reason=NoTelemetrySource` | Aucune source de télémétrie réelle configurée (ou `AIGateway` introuvable/mal référencée). | `AIGateway.spec.telemetry.mode` ; en `configmap`, la ConfigMap `usage.json` existe-t-elle dans le bon namespace ? |
| `Ready=False`, `reason=ReferenceNotFound` | `providerRef`/`gatewayRef`/`fallbackModelRef` pointe vers un objet absent. | `kubectl get aiprov,aimodel,aigw` dans le même namespace ; l'ordre de création n'a pas d'importance (les controllers ré-enfilent au watch). |
| `AIBudgetPolicy` en phase `Unknown` | `budgetEUR` non défini ou `<= 0`. | `kubectl get aibudget <nom> -o jsonpath='{.status.message}'` — message explicite "cannot evaluate". |
| Fallback budgétaire jamais actué malgré `enforce` | Une des conditions de garde listées en [section 8](#8-enforcement-et-fallback-budgétaire) échoue. | `kubectl get aibudget <nom> -o jsonpath='{.status.fallbackReason}'` explique exactement laquelle. |
| `AIQualityGate` reste `Pending`, verdict `insufficient-data` | Golden dataset/evidence insuffisants, ou une dimension à poids non nul n'a aucun signal. | `status.failureMessages[]` liste le signal manquant précisément (règle anti-fabrication, section 4). |
| Pas de findings shadow-AI malgré du trafic direct suspecté | Pas de ConfigMap `shadow-egress` (clé `egress.json`) dans le namespace de la policy, ou host non reconnu comme endpoint LLM connu. | La détection ignore silencieusement les hosts non catalogués — ce n'est pas un firewall. |
| `ai_finops_*` absentes de Prometheus | ServiceMonitor non activé, ou label `release` ne correspondant pas à la sélection de l'opérateur Prometheus. | `--set metrics.serviceMonitor.enabled=true --set metrics.serviceMonitor.labels.release=<nom-release-prometheus>`. |

## Intégration avec d'autres opérateurs (optionnelle)

- **ai-govar-operator** : peut lire le catalogue (`AIModel`/`AIProvider`/`AIRoutingPolicy`)
  et le workflow `AIChangeRequest` (action `authorize-gov-ar-route`) de cet opérateur. Ses
  webhooks fail-closed tamponnent l'identité des reviewers d'approbation. Sans lui, le
  workflow `reroute` fonctionne normalement — cette intégration est strictement optionnelle.
- **ai-confidential-operator** : aucune dépendance croisée, les deux opérateurs sont
  totalement indépendants.

> Ne pas installer deux opérateurs qui réconcilient les mêmes CRDs sur le même cluster.

## Statut

Validé bout en bout sur kind (2026-07-20) : CRs réconciliées (budget en phase `Exceeded`
à 697 %, 3 constats de souveraineté), rapport Markdown généré en ConfigMap, 52 métriques
`ai_finops_*` exposées et scrapées, dashboard Grafana importé avec ses 22 panels alimentés.
`go build ./...` et `go vet ./...` passent sans erreur ; suite de tests unitaires verte sur
tous les moteurs purs (`internal/*engine`, `pkg/qualitystats`).

## Contribuer

Les issues et pull requests sont bienvenues. Avant de proposer un changement de comportement
(formule de score, seuil par défaut, nouveau champ de CRD), merci d'ouvrir une issue pour en
discuter — ce dépôt documente précisément ce qu'il calcule, donc tout changement de formule
est un changement de contrat public. Merci de faire passer `go build ./...`, `go vet ./...`
et les tests concernés avant de soumettre une PR.

## License

Distribué sous licence **Apache License 2.0** — voir [LICENSE](LICENSE).
