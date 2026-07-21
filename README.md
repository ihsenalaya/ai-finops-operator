# AI FinOps Operator

Opérateur Kubernetes de gouvernance FinOps & souveraineté du trafic IA : attribution des
coûts (EUR) par requête/modèle/équipe/namespace, budgets avec dégradation gracieuse et
fallback managé, contraintes de résidence des données, point mort managé/auto-hébergé,
quality gates par application, optimisation de routage et workflow d'approbation humaine.

Il possède **11 CRDs** sous le groupe API `aiops.imperium.io/v1alpha1` et s'installe et
fonctionne de manière autonome — aucune dépendance à l'exécution sur un autre opérateur.

## Contenu du dépôt

```
ai-finops-operator/
├── README.md                      ← ce fichier
├── api/v1alpha1/                  ← types des 11 CRDs
├── internal/controller/           ← 11 reconcilers
├── cmd/                           ← finops-manager, header-proxy, seed-usage
├── charts/ai-finops-operator/     ← Helm chart (11 CRDs + RBAC scopé)
├── docs/                          ← une fiche par CRD
└── automatisation/
    ├── up.sh / down.sh            ← cluster kind complet en une commande
    ├── test-apps/                 ← catalogue (01), usage gateway (02), policies+rapport (03),
    │                                 quality gate (04), egress shadow-AI (05), gateway d'éval (06)
    └── dashboards/                ← dashboard Grafana "AI FinOps Operator — Overview"
```

## Fonctionnement

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
   AIBreakEvenAnalysis  │ 4. Publication                               │
                        │    status CRs + events + métriques ai_finops_│
                        │    AIFinOpsReport → ConfigMap (md/json)      │
                        │ 5. Enforcement (opt-in, mode enforce)        │
                        │    reroute budget / blocage souveraineté     │
                        └──────────────────────────────────────────────┘
```

1. **Collecte** — `AIGateway` déclare la source de télémétrie (`prometheus`,
   `configmap`, `aigw`). Il n'existe **aucun repli silencieux** : sans source réelle,
   l'opérateur pose la condition `NoTelemetrySource` au lieu d'inventer des données.
2. **Valorisation** — chaque enregistrement d'usage (application, modèle, tokens
   entrée/sortie) est converti en EUR avec les tarifs du catalogue `AIProvider`/`AIModel`,
   puis ventilé par namespace, application, modèle et **zone de résidence** (EU/US…).
3. **Évaluation des politiques** — à chaque réconciliation :
   - `AIBudgetPolicy` compare la dépense observée au budget de la période et publie
     `usage%` et une phase (`Ok` → `Warning` → `Exceeded`), avec seuils d'alerte ;
   - `AISovereigntyPolicy` confronte la zone de chaque provider utilisé aux zones
     autorisées et produit des **findings** par application (sévérité incluse) ;
   - `AIBreakEvenAnalysis` calcule le point mort API managée vs auto-hébergement
     (coût GPU) et les économies (positives ou négatives) du basculement.
4. **Publication** — statuts typés sur chaque CR (`kubectl get aibudget` montre la
   phase), events Kubernetes, métriques `ai_finops_*`, et `AIFinOpsReport` qui génère un
   **ConfigMap** avec le rapport complet en Markdown + JSON (résumé exécutif, coût par
   modèle/application, projection mensuelle run-rate, recommandations).
5. **Enforcement (opt-in)** — `enforcementMode: reportOnly → warn → enforce`. En
   `enforce` avec une Envoy AI Gateway configurée, l'opérateur actue réellement :
   reroute budget vers le fallback managé conforme, blocage souveraineté, reroute manuel
   immédiat (`AIRouteOverride`) et changement gouverné par approbation humaine
   (`AIChangeRequest`).
6. **Optimisation continue** — `AIRoutingPolicy` score chaque route candidate
   (coût, latence, qualité — `ai_finops_routing_score`) ; `AIQualityGate` valide la
   qualité d'un modèle candidat **avant** toute bascule ; le radar qualité/coût du
   dashboard compare les fournisseurs sur ces dimensions.

## Fonctionnalités

- **Attribution des coûts en EUR** par requête, modèle, application, équipe, namespace
  et zone — à partir d'une télémétrie mesurée, jamais simulée silencieusement.
- **Budgets avec dégradation gracieuse** : seuils d'alerte, phase `Exceeded`, fallback
  managé conforme au lieu d'un blocage brutal.
- **Souveraineté du trafic IA** : contraintes de résidence des données (ex. FR/EU),
  findings par application, blocage optionnel du trafic hors-zone.
- **Break-even managé vs auto-hébergé** : à quel volume un H100 auto-hébergé devient
  rentable face à l'API managée (économies mensuelles chiffrées).
- **Rapports FinOps consolidés** : ConfigMap Markdown/JSON par namespace et par période,
  consommable par un humain ou un pipeline.
- **Quality gates** : validation multi-dimensions (correctness, reliability, latency,
  semantic) avant changement de modèle.
- **Optimisation de routage** + **reroute manuel réversible** + **workflow
  d'approbation humaine** pour les changements sensibles.
- **Détection shadow-AI** : trafic IA sortant qui contourne la gateway
  (`ai_finops_shadow_ai_egress`).
- **Observabilité native** : ~30 familles de métriques `ai_finops_*`, events, conditions.

## CRDs possédées (11)

Groupe API : `aiops.imperium.io/v1alpha1`.

| CRD | shortName | Rôle | Doc |
|---|---|---|---|
| AIProvider | `aiprov` | Fournisseur IA : tarifs, zone de résidence, conformité | [docs/aiprovider.md](docs/aiprovider.md) |
| AIModel | — | Modèle catalogué lié à un provider | [docs/aimodel.md](docs/aimodel.md) |
| AIGateway | `aigw` | Gateway observée + mode de télémétrie | [docs/aigateway.md](docs/aigateway.md) |
| AIBudgetPolicy | `aibudget` | Budget + seuils + fallback managé | [docs/aibudgetpolicy.md](docs/aibudgetpolicy.md) |
| AISovereigntyPolicy | `aisov` | Résidence des données / données sensibles | [docs/aisovereigntypolicy.md](docs/aisovereigntypolicy.md) |
| AIBreakEvenAnalysis | `aibreakeven` | Point mort API managée vs auto-hébergement | [docs/aibreakevenanalysis.md](docs/aibreakevenanalysis.md) |
| AIFinOpsReport | `aireport` | Rapport consolidé (Markdown/JSON en ConfigMap) | [docs/aifinopsreport.md](docs/aifinopsreport.md) |
| AIQualityGate | `aiqgate` | Validation qualité avant changement de modèle | [docs/aiqualitygate.md](docs/aiqualitygate.md) |
| AIRoutingPolicy | `airpolicy` | Politique d'optimisation continue du routage | [docs/airoutingpolicy.md](docs/airoutingpolicy.md) |
| AIRouteOverride | `airoverride` | Reroute manuel immédiat (réversible) | [docs/airouteoverride.md](docs/airouteoverride.md) |
| AIChangeRequest | `aicrq` | Demande de changement gouvernée (approbation humaine) | [docs/aichangerequest.md](docs/aichangerequest.md) |

Le manager (`cmd/finops-manager`) enregistre exactement les 11 controllers de ces
CRDs — pas de webhook, pas d'écriture hors des chemins d'enforcement déclarés.

## Installation

### Prérequis

- Kubernetes ≥ 1.29, Helm ≥ 3.12.
- *Optionnel* : Prometheus Operator (ServiceMonitor), Grafana, une gateway IA
  (Envoy AI Gateway, LiteLLM…) comme plan de données pour l'enforcement réel.

### Depuis le registre d'images publié

```bash
helm install finops ./charts/ai-finops-operator \
  --namespace finops-system --create-namespace
```

L'image par défaut est `ghcr.io/ihsenalaya/ai-finops-operator/finops-operator`
(tag = `appVersion` du chart). Valeurs utiles :

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

## Utilisation

### Boucle FinOps minimale

1. Déclarer le catalogue : `AIProvider` (tarifs + zone) et `AIModel`.
2. Pointer la télémétrie : `AIGateway` (`prometheus`, `configmap`, `aigw` — jamais de repli
   `fake` silencieux : sans source réelle, condition `NoTelemetrySource` explicite).
3. Poser les politiques : `AIBudgetPolicy`, `AISovereigntyPolicy`.
4. Consommer : `AIFinOpsReport` (ConfigMap Markdown/JSON), métriques `ai_finops_*`, events.

### Enforcement

`enforcementMode` sur les politiques : `reportOnly` → `warn` → `enforce`. En mode `enforce`
avec une Envoy AI Gateway, l'opérateur **actue réellement** : reroute budget vers le fallback
managé conforme, blocage souveraineté par backend réservé, `AIRouteOverride` immédiat, et
`AIChangeRequest` pour le changement gouverné par un humain.

### Métriques

Famille `ai_finops_*` sur `:8080/metrics` : coûts (`ai_finops_cost_eur`,
`ai_finops_cost_by_zone_eur`), budgets (`ai_finops_budget_usage_percent`), souveraineté
(`ai_finops_sovereignty_findings`), enforcement (`ai_finops_enforcement_actions`),
optimisation (`ai_finops_potential_savings_eur`, `ai_finops_routing_score`), qualité
(`ai_finops_quality_score`, `ai_finops_quality_gate_passed`), shadow-AI
(`ai_finops_shadow_ai_egress`).

## Statut

Validé bout en bout sur kind (2026-07-20) : CRs réconciliées (budget en phase `Exceeded`
à 697 %, 3 constats de souveraineté), rapport Markdown généré en ConfigMap, 52 métriques
`ai_finops_*` exposées et scrapées, dashboard Grafana importé avec ses 22 panels alimentés.

## Intégration avec d'autres opérateurs (optionnelle)

- **ai-govar-operator** : peut lire le catalogue (`AIModel`/`AIProvider`/`AIRoutingPolicy`)
  et le workflow `AIChangeRequest` (action `authorize-gov-ar-route`) de cet opérateur. Ses
  webhooks fail-closed tamponnent l'identité des reviewers d'approbation. Sans lui, le
  workflow `reroute` fonctionne normalement — cette intégration est strictement optionnelle.
- **ai-confidential-operator** : aucune dépendance croisée, les deux opérateurs sont
  totalement indépendants.

> Ne pas installer deux opérateurs qui réconcilient les mêmes CRDs sur le même cluster.
