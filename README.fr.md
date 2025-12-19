# azdo-agent-operator

Un opérateur Kubernetes qui automatise la gestion du cycle de vie des agents Azure DevOps. Il provisionne, adapte l'échelle et supprime dynamiquement les pods agents en fonction de la demande de vos pipelines Azure DevOps.

## Description

L'**azdo-agent-operator** simplifie le processus d'intégration et de livraison continues en intégrant Azure DevOps à votre cluster Kubernetes. Il surveille la file d'attente des tâches de vos pools d'agents Azure DevOps et gère automatiquement les pods agents pour traiter les travaux de build et de release en attente. En alignant dynamiquement votre infrastructure sur la charge de travail actuelle, l'opérateur réduit l'intervention manuelle et optimise l'utilisation des ressources. Il prend en charge les environnements Azure DevOps Services et Azure DevOps Server et est conçu pour une haute scalabilité, ce qui en fait une solution idéale pour les équipes souhaitant moderniser leurs pipelines CI/CD.

## Prérequis

- **Go** : version v1.23.0+
- **Docker** : version 17.03+
- **kubectl** : version v1.11.3+
- Accès à un cluster Kubernetes (v1.11.3+)

## Démarrage

### Déploiement sur le Cluster

**Construisez et poussez votre image vers l'emplacement spécifié par `IMG` :**

```sh
make docker-build docker-push IMG=<some-registry>/azdo-agent-operator:tag
```

> **REMARQUE** : Cette image doit être publiée dans le registre spécifié, et votre cluster doit avoir la permission de tirer les images de ce registre. Vérifiez que vous disposez des autorisations appropriées en cas de problèmes.

**Installez les CRD dans le cluster :**

```sh
make install
```

**Déployez le Manager sur le cluster avec l'image spécifiée par `IMG` :**

```sh
make deploy IMG=<some-registry>/azdo-agent-operator:tag
```

> **REMARQUE** : Si vous rencontrez des erreurs RBAC, il peut être nécessaire de vous accorder des privilèges d'administrateur du cluster ou de vous assurer que vous êtes connecté en tant qu'administrateur.

### Création d'Instances de Votre Solution

Vous pouvez appliquer les ressources personnalisées (CR) d'exemple depuis le répertoire `config/samples` :

```sh
kubectl apply -k config/samples/
```

> **REMARQUE** : Assurez-vous que les configurations d'exemple ont les valeurs par défaut appropriées pour tester l'opérateur.

### Désinstallation

**Supprimez les instances (CR) du cluster :**

```sh
kubectl delete -k config/samples/
```

**Supprimez les API (CRD) du cluster :**

```sh
make uninstall
```

**Retirez le contrôleur du cluster :**

```sh
make undeploy
```

## Distribution du Projet

Il existe deux principales méthodes pour distribuer et déployer l'azdo-agent-operator.

### Fournir un Bundle avec Tous les Fichiers YAML

1. **Construisez l'installateur pour l'image :**

   Générez un bundle d'installation en utilisant :

   ```sh
   make build-installer IMG=<some-registry>/azdo-agent-operator:tag
   ```

   Cette commande crée un fichier `install.yaml` dans le répertoire `dist`. Ce fichier contient toutes les ressources Kubernetes générées avec Kustomize nécessaires pour installer l'opérateur.

2. **Utilisation de l'installateur :**

   Les utilisateurs peuvent installer l'opérateur en appliquant directement le bundle YAML :

   ```sh
   kubectl apply -f https://raw.githubusercontent.com/<org>/azdo-agent-operator/<tag-or-branch>/dist/install.yaml
   ```

### Fournir un Chart Helm

1. **Construisez le Chart Helm en utilisant le plugin Helm optionnel :**

   ```sh
   kubebuilder edit --plugins=helm/v1-alpha
   ```

2. **Localisez le Chart :**

   Un Chart Helm sera généré sous `dist/chart`. Les utilisateurs peuvent installer ou empaqueter l'opérateur en utilisant le workflow standard de Helm.

   > **REMARQUE** : Lorsque des modifications sont apportées au projet, mettez à jour le Chart Helm avec la même commande. Si vous ajoutez des webhooks ou d'autres configurations, assurez-vous que les paramètres personnalisés dans `dist/chart/values.yaml` ou `dist/chart/manager/manager.yaml` sont réappliqués manuellement si nécessaire.

## Fonctionnalités prévues / feuille de route

L'opérateur continue d'évoluer. Les fonctionnalités suivantes sont planifiées et suivies dans les issues GitHub :

- **Plans de pools et de files d'attente Azure DevOps déclaratifs**  
  Des CRD `AzdoAgentPool` et `AzdoAgentQueue` pour créer et réconcilier les pools et files d'attente ADO directement depuis des manifestes Kubernetes (voir issues #43, #44, #45).

- **Cycle de vie des agents éphémères et sécurisé**  
  Agents à un seul job, drain lors des suppressions, TTL/nettoyage des « zombies » et meilleure gestion des pods/agents distants supprimés (voir issues #28, #39, #47).

- **Autoscaling piloté par la demande**  
  Scalabilité basée sur la longueur de file d'attente avec « warm pool », politiques d'autoscaling et routage par profil/demandes plutôt que « un pool par image » (voir issues #25, #29, #40, #41).

- **Observabilité et télémétrie**  
  Endpoints de métriques pour l'opérateur, support OpenTelemetry optionnel et exposition de métriques Azure DevOps au format Prometheus (voir issues #22, #23, #24, #31, #32, #36, #38).

- **Sécurité, authentification et secrets**  
  Modes d'authentification plus propres (PAT, managed identity, workload identity), intégrations avec des gestionnaires de secrets et flags globaux de sécurité pour le nettoyage/scaling (voir issues #19, #20, #21, #33, #42, #48).

- **Expérience développeur et documentation**  
  Quickstarts améliorés, diagrammes d'architecture, exemples de RBAC et outils de développement local (voir issues #30, #34, #35, #36, #37, #38).

Vous pouvez suivre ou discuter ces sujets directement sur la [page des issues GitHub](https://github.com/Simplifi-ED/azdo-kube-operator/issues).

## Mainteneur & société

Ce projet est maintenu par **Etienne Deneuve** chez [Omnivya](https://www.omnivya.fr).

- Site web : https://etienne.deneuve.xyz
- LinkedIn : https://www.linkedin.com/in/etiennedeneuve/

## Contribution

Nous accueillons avec plaisir les contributions de la communauté ! Si vous souhaitez aider à améliorer l'azdo-agent-operator, veuillez suivre ces directives :

- **Fork du Dépôt** : Créez un fork personnel et travaillez sur une branche dédiée.
- **Normes de Codage** : Suivez les standards de codage du projet, y compris les tests unitaires (TDD) et les tests d'intégration lorsque cela est applicable.
- **Pull Requests** : Soumettez des pull requests avec des descriptions claires de vos modifications. Assurez-vous que tous les tests passent avant de soumettre.
- **Issues** : Si vous trouvez des bugs ou avez des suggestions de fonctionnalités, veuillez ouvrir une issue avec une explication détaillée.

Pour plus d'informations, référez-vous à notre `CONTRIBUTING.md` (si disponible) et à la documentation de Kubebuilder pour les meilleures pratiques de développement d'opérateurs.

> **REMARQUE** : Exécutez `make help` pour obtenir la liste de toutes les cibles make disponibles et des commandes supplémentaires du projet.

## Licence

Copyright 2025.

Sous licence Apache, version 2.0 (la "Licence") ; vous ne pouvez pas utiliser ce fichier sauf en conformité avec la Licence.
