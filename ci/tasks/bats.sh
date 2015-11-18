#!/usr/bin/env bash

set -e -x

source bosh-cpi-release/ci/tasks/utils.sh

check_param BOSH_DIRECTOR_PUBLIC_IP
check_param BOSH_DIRECTOR_PRIVATE_IP
check_param PRIMARY_NETWORK_CIDR
check_param PRIMARY_NETWORK_GATEWAY
check_param PRIMARY_NETWORK_RANGE
check_param PRIMARY_NETWORK_MANUAL_IP
check_param SECONDARY_STATIC_IP
check_param BAT_SPEC

base_dir=${PWD}
keys_dir=${base_dir}/keys
mkdir -p ${keys_dir}
eval $(ssh-agent)

bosh_ssh_key="${keys_dir}/bats.pem"
ssh-keygen -N "" -t rsa -b 4096 -f ${bosh_ssh_key}
chmod go-r ${bosh_ssh_key}
ssh-add ${bosh_ssh_key}
mkdir -p ~/.ssh/id_rsa.pub
cp ${base_dir}/keys/bats.pem.pub ~/.ssh/id_rsa.pub

vm_ssh_key="${keys_dir}/vm"
vm_ssh_public_key="${keys_dir}/vm.pub"
ssh-keygen -N "" -t rsa -b 4096 -f ${vm_ssh_key}
chmod go-r ${vm_ssh_key}
vm_ssh_public_key_base64=$(cat ${vm_ssh_public_key} | tr -d '\n' | base64)

cd bats

# checked by BATs environment helper (bosh-acceptance-tests.git/lib/bat/env.rb)
export BAT_DIRECTOR=${BOSH_DIRECTOR_PUBLIC_IP}
export BAT_STEMCELL=${base_dir}/stemcell/stemcell.tgz
export BAT_DEPLOYMENT_SPEC="${PWD}/bats-config.yml"
export BAT_VCAP_PASSWORD='c1oudc0w'
export BAT_DNS_HOST=${BOSH_DIRECTOR_PUBLIC_IP}
export BAT_INFRASTRUCTURE='rackhd'
export BAT_NETWORKING='manual'
export BAT_VCAP_PRIVATE_KEY=${vm_ssh_key}

echo "using bosh CLI version..."
bosh version

echo "targeting bosh director at ${BOSH_DIRECTOR_PUBLIC_IP}"
bosh -n target ${BOSH_DIRECTOR_PUBLIC_IP}

cat > ${BAT_DEPLOYMENT_SPEC} <<EOF
---
cpi: rackhd
properties:
  key_name:  bats
  use_static_ip: true
  second_static_ip: ${SECONDARY_STATIC_IP}
  public_key: ${vm_ssh_public_key_base64}
  pool_size: 1
  instances: 1
  uuid: $(bosh status --uuid)
  stemcell:
    name: bosh-openstack-kvm-ubuntu-trusty-go_agent-raw
    version: latest
  networks:
  - name: default
    static_ip: ${PRIMARY_NETWORK_MANUAL_IP}
    type: manual
    cidr: ${PRIMARY_NETWORK_CIDR}
    reserved: [${BOSH_DIRECTOR_PRIVATE_IP}]
    static: [${PRIMARY_NETWORK_RANGE}]
    gateway: ${PRIMARY_NETWORK_GATEWAY}
EOF

./write_gemfile
rm Gemfile.lock
bundle install

# create dev release
pushd ${PWD}/spec/system/assets/bat-release
rm -rf dev_releases
bosh create release --force --name=bat
cp dev_releases/bat/* dev_releases/
bosh --user admin --password admin upload release
popd

echo "running the tests"
bundle exec rspec ${BAT_SPEC}

