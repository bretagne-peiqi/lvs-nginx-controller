#!/bin/bash
#
# The script builds lvs-nginx-controller component container, see usage function for how to run
# the script. After build completes, following container will be built, i.e.
#   telecom/lvs-controller:${IMAGE_TAG}
#
# By default, IMAGE_TAG is latest.

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")/..

function usage {
  echo -e "Usage:"
  echo -e "  ./build-image.sh [tag]"
  echo -e ""
  echo -e "Parameter:"
  echo -e " tag\tDocker image tag, treated as lvs-nginx-controller release version. If provided,"
  echo -e "    \tthe tag must be the form of vA.B.C, where A, B, C are digits, e.g."
  echo -e "    \tv1.0.1. If not provided, it will build images with tag 'latest'"
  echo -e ""
  echo -e "Environment variable:"
  echo -e " PUSH_TO_REGISTRY     \tPush images to telecom registry or not, options: Y or N. Default value: ${PUSH_TO_REGISTRY}"
}

# -----------------------------------------------------------------------------
# Parameters for building docker image, see usage.
# -----------------------------------------------------------------------------
# Decide if we need to push the new images to telecom registry.
PUSH_TO_REGISTRY=${PUSH_TO_REGISTRY:-"N"}

# Find image tag version, the tag is considered as release version.
if [[ "$#" == "1" ]]; then
  if [[ "$1" == "help" || "$1" == "--help" || "$1" == "-h" ]]; then
    usage
    exit 0
  else
    IMAGE_TAG=${1}
  fi
else
  IMAGE_TAG="latest"
fi

echo "+++++ Start building lvs-nginx-controller"

cd ${ROOT}

# Build lvs-nginx-controller binary.
docker run --rm -v `pwd`:/usr/local/go/src/github.com/lvs-nginx-controller/cmd/lvs-controller docker-registry.telecom.com/base/golang:1.10 sh -c "cd /var/root/go/src/github.com/lvs-nginx-controller/cmd/lvs-controller  && go build -race ."
# Build lvs-nginx-controller container.
docker build -t telecom/lvs-controller:${IMAGE_TAG} .
docker tag telecom/lvs-controller:${IMAGE_TAG} docker-registry.telecom.com/kubernetes-lb/lvs-controller:${IMAGE_TAG}

cd - > /dev/null

# Decide if we need to push images to telecom registry.
if [[ "$PUSH_TO_REGISTRY" == "Y" ]]; then
  echo ""
  echo "+++++ Start pushing lvs-controller"
  docker push docker-registry.telecom.com/kubernetes-lb/lvs-controller:${IMAGE_TAG}
fi

echo "Successfully built docker image telecom/lvs-controller:${IMAGE_TAG}"
echo "Successfully built docker image docker-registry.telecom.com/kubernetes-lb/lvs-controller:${IMAGE_TAG}"

# A reminder for creating Github release.
if [[ "$#" == "1" && $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo -e "Finish building release ; if this is a formal release, please remember"
  echo -e "to create a release tag at Github at: https://github.com/peiqi-telecom/lvs-nginx-controller/releases"
fi
