# CRD — AIProvider

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aiprov`.

Décrit un fournisseur de modèles (API managée ou GPU auto-hébergé) avec son **pricing** et ses
attributs de **souveraineté/conformité**.

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `type` | enum `openai\|azure-openai\|mistral\|anthropic\|bedrock\|vertex\|self-hosted\|custom` | ✔ | Famille de fournisseur. |
| `region` | string | | Région cloud (ex. `francecentral`). |
| `dataResidency` | string | | Zone de traitement (ex. `france`, `eu`, `us`). |
| `managed` | bool | | `true` = API managée, `false` = auto-hébergé GPU. |
| `pricing.currency` | string | ✔ (défaut `EUR`) | Devise des prix. |
| `pricing.inputTokenPricePerMillion` | Quantity | ✔ | Prix / M tokens input. |
| `pricing.outputTokenPricePerMillion` | Quantity | ✔ | Prix / M tokens output. |
| `pricing.fixedMonthlyCost` | Quantity | | Frais fixes mensuels (commitments…). |
| `compliance.allowedForSensitiveData` | bool | | Donnée sensible autorisée. |
| `compliance.allowedCountries[]` | []string | | Pays/zones servis. |

> Les montants sont des `resource.Quantity` (décimaux exacts, ex. `"2.5"`), idiomatiques k8s.

## Status
`observedGeneration`, `conditions[]` (`Ready`).

## Comportement du controller
Valide la définition (le schéma OpenAPI impose la structure), pose `Ready=True`. Le pricing est
consommé par [costengine](../features/costengine.md) et [breakevenengine](../features/breakevenengine.md) ;
la résidence par [sovereigntyengine](../features/sovereigntyengine.md).

## Exemple
[`config/samples/aiops_v1alpha1_aiprovider.yaml`](../../config/samples/aiops_v1alpha1_aiprovider.yaml),
[`..._aiprovider_openai_us.yaml`](../../config/samples/aiops_v1alpha1_aiprovider_openai_us.yaml).

Lié : [AIModel](aimodel.md).
