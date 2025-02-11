FROM docker:dind-rootless

# Passer en root pour installer les packages
USER root

RUN apk add --no-cache fuse-overlayfs shadow

# Définir la variable pour forcer l'utilisation de fuse-overlayfs comme snapshotter
ENV BUILDKIT_SNAPSHOTTER=fuse-overlayfs

# Si nécessaire, repasser à l'utilisateur non-root (généralement 1000 pour dind-rootless)
USER 1000
