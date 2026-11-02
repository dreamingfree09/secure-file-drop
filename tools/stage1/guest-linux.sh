#!/bin/bash
# Only inside the new managed Linux VM, after offline OS installation.
set -euo pipefail
umask 077
[ "$#" -eq 2 ] || exit 2
identity=$1
input=$2
[ "$(id -u)" = 0 ] || { echo 'STOP:guest-root-required'; exit 2; }
python3 -I -B - "$identity" <<'PY'
import json,pathlib,sys,subprocess
v=json.loads(pathlib.Path(sys.argv[1]).read_text())
u=pathlib.Path('/sys/class/dmi/id/product_uuid').read_text().strip().lower()
assert v['schema']=='SFD_STAGE1_GUEST_V1' and v['role']=='linux' and v['work_id']=='SFD-STAGE1-INSTALLATION-20261102'
assert v['bios_uuid'].strip('{}').lower()==u
m=pathlib.Path('/proc/self/mountinfo').read_text().lower()
assert not any(x in m for x in [' - 9p ',' - drvfs ',' - cifs ',' - nfs '])
r=json.loads(subprocess.check_output(['ip','-j','route','show','default']));assert not r
PY
[ ! -S /var/run/docker.sock ] || { echo 'STOP:unowned-engine-socket'; exit 2; }
[ ! -e /srv/sfd-lab ] || { echo 'STOP:preserve-existing-state'; exit 2; }
# Verify source-bound package checksums/signatures OFFLINE before any package
# scripts. No convenience script or online apt fallback is permitted.
(cd "$input"; sha256sum --strict --check SHA256SUMS >/dev/null)
python3 -I -B - "$input" <<'PYVERIFY'
import hashlib,json,pathlib,subprocess,sys
p=pathlib.Path(sys.argv[1]);lock=json.loads((p/'runtime-lock.json').read_text())
# Official apt signing verification is performed over InRelease and Packages
# before media creation, then bound by Aether's exact bundle review. Recheck
# packet hashes, lengths, package names/versions and absence of extra scripts.
r=json.loads((p/'SIGNED-PACKAGES-VERIFIED.json').read_text())
assert r['schema']=='SFD_STAGE1_SIGNED_PACKAGES_V1' and r['official_signatures_verified'] is True
assert r['source_review_complete'] is True and len(r['packages'])>=5
expected={x['Package']:x for x in lock['Engine_packages'].values()}
seen=set()
for row in r['packages']:
    f=p/'packages'/row['filename'];assert f.parent==p/'packages' and not f.is_symlink()
    raw=f.read_bytes();assert len(raw)==row['bytes'] and hashlib.sha256(raw).hexdigest()==row['sha256']
    fields=subprocess.check_output(['dpkg-deb','--show','--showformat=${Package}\n${Version}\n${Architecture}',str(f)]).decode().splitlines()
    assert fields==[row['package'],row['version'],'amd64'] and row['package'] not in seen;seen.add(row['package'])
    if row['package'] in expected:
        e=expected[row['package']];assert row['version']==e['Version'] and row['sha256']==e['SHA256'] and row['bytes']==int(e['Size'])
assert set(expected)<=seen
assert {f.name for f in (p/'packages').glob('*.deb')}=={x['filename'] for x in r['packages']}
PYVERIFY
# One private NIC and static address; no external NIC, DHCP, gateway or DNS.
python3 -I -B - <<'PYNET'
import json,subprocess
links=json.loads(subprocess.check_output(['ip','-j','link']))
links=[x for x in links if x['ifname']!='lo'];assert len(links)==1 and links[0]['address'].lower()=='00:15:5d:a1:b7:01'
PYNET
[ ! -e /etc/netplan/99-sfd-lab.yaml ] || { echo 'STOP:preserve-existing-network-file'; exit 2; }
cat >/etc/netplan/99-sfd-lab.yaml <<'EOF'
network:
  version: 2
  ethernets:
    sfdprivate:
      match: {macaddress: "00:15:5d:a1:b7:01"}
      set-name: sfdprivate
      dhcp4: false
      dhcp6: false
      accept-ra: false
      link-local: []
      addresses: [172.31.250.1/30]
EOF
chmod 600 /etc/netplan/99-sfd-lab.yaml
netplan generate
netplan apply
install -d -m700 /srv/sfd-lab /srv/sfd-lab/private /srv/sfd-lab/public /etc/sfd-lab/docker /etc/sfd-lab/containerd
# Block postinst auto-start in this FRESH guest only; restore policy only after
# packages have been configured and all daemon/store identities verified.
[ ! -e /usr/sbin/policy-rc.d ] || { echo 'STOP:preserve-existing-service-policy'; exit 2; }
printf '#!/bin/sh\nexit 101\n' >/usr/sbin/policy-rc.d
chmod 755 /usr/sbin/policy-rc.d
for unit in docker.service docker.socket containerd.service; do systemctl mask "$unit"; done
# dpkg failure is preserved; never repair it by fetching unreviewed packages.
dpkg --install "$input"/packages/*.deb >/srv/sfd-lab/private/dpkg.log 2>&1
cat >/etc/sfd-lab/containerd/config.toml <<'EOF'
version = 2
root = "/var/lib/sfd-lab/containerd"
state = "/run/sfd-lab/containerd"
[grpc]
  address = "/run/sfd-lab/containerd/containerd.sock"
EOF
cat >/etc/sfd-lab/docker/daemon.json <<'EOF'
{"data-root":"/var/lib/sfd-lab/docker","exec-root":"/run/sfd-lab/docker","pidfile":"/run/sfd-lab/dockerd.pid","hosts":["unix:///run/sfd-lab/docker.sock"],"containerd":"/run/sfd-lab/containerd/containerd.sock","containerd-namespace":"sfd-lab","containerd-plugins-namespace":"sfd-lab-plugins","live-restore":false,"log-driver":"json-file","log-opts":{"max-size":"1m","max-file":"3"}}
EOF
# No default unit is unmasked. These are explicit new-guest-only transient units.
systemd-run --unit=sfd-lab-containerd --property=Restart=no /usr/bin/containerd --config /etc/sfd-lab/containerd/config.toml
systemd-run --unit=sfd-lab-dockerd --property=Restart=no /usr/bin/dockerd --config-file /etc/sfd-lab/docker/daemon.json
while IFS='=' read -r name value; do case "$name" in DOCKER_*|BUILDX_*|COMPOSE_*) unset "$name";; esac; done < <(env)
export DOCKER_HOST=unix:///run/sfd-lab/docker.sock
[ "$(dockerd --version | cut -d' ' -f3 | tr -d ',')" = 29.8.2 ] || { echo 'STOP:engine-version'; exit 2; }
containerd --version | grep -Eq ' v?2\.4\.2 ' || { echo 'STOP:containerd-version'; exit 2; }
for i in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
[ "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/sfd-lab/docker ] || { echo 'STOP:daemon-root'; exit 2; }
# The checksum-bound export contains only newly built/accepted lab images.
docker load --input "$input/runtime-images.tar" >/srv/sfd-lab/private/import.log
python3 -I -B "$input/verify-import.py" "$input/runtime-receipt.json"
echo 'NEW GUEST ENGINE PREPARED. No application/issuer/trust start authorised by this bootstrap.'
