#!/usr/bin/env bash
# Creates demo resources through the state-manager API (never directly in the DB).
set -euo pipefail
SM="${STATE_MANAGER_URL:-http://127.0.0.1:58080}"
N="${SEED_COUNT:-40}"

post() { curl -sf -o /dev/null -X "$1" -H 'Content-Type: application/json' "$SM$2" -d "$3"; }

groups=(jobs.example.io compute.example.io network.example.io storage.example.io)
kinds_jobs=(Job CronJob)
kinds_compute=(VirtualMachine Disk Image)
kinds_network=(Network SecurityGroup)
kinds_storage=(Bucket)
envs=(prod stage dev)
teams=(core platform data)
phases=(Pending Running Succeeded Failed)

created=0
for ((i = 1; i <= N; i++)); do
  for g in "${groups[@]}"; do
    case "$g" in
      jobs.*) kinds=("${kinds_jobs[@]}") ;;
      compute.*) kinds=("${kinds_compute[@]}") ;;
      network.*) kinds=("${kinds_network[@]}") ;;
      *) kinds=("${kinds_storage[@]}") ;;
    esac
    k="${kinds[$((i % ${#kinds[@]}))]}"
    ns="project-$((i % 5 + 1))"
    shard="shard-$((i % 3 + 1))"
    env="${envs[$((i % 3))]}"
    team="${teams[$(((i / 3) % 3))]}"
    name="$(echo "$k" | tr '[:upper:]' '[:lower:]')-$(printf '%03d' "$i")"
    meta=$(cat <<JSON
"resource_group":"$g","kind":"$k","namespace":"$ns","name":"$name","shard_id":"$shard",
 "labels":{"env":"$env","team":"$team","app":"svc-$((i % 7))"},
 "annotations":{"app.kubernetes.io/name":"svc-$((i % 7))","owner":"$team@example.com"}
JSON
)
    body=$(cat <<JSON
{$meta,
 "spec":{"image":"registry.local/svc-$((i % 7)):1.$((i % 4)).0","replicas":$((i % 6)),"resources":{"cpu":$((i % 8 + 1)),"memory":"$((i % 4 + 1))Gi"}}}
JSON
)
    path="/api/v1/groups/$g/namespaces/$ns/kinds/$k/resources"
    if post POST "$path/" "$body"; then
      created=$((created + 1))
      # Most resources get a status as if reconciled by an operator.
      # The status update replaces labels/annotations too, so they are sent again.
      if ((i % 4 != 0)); then
        phase="${phases[$((i % 4))]}"
        exit_code=0
        [[ "$phase" == Failed ]] && exit_code=$((i % 3 + 1))
        status=$(cat <<JSON
{$meta,
 "status":{"phase":"$phase","exit_code":$exit_code,"conditions":[{"type":"Ready","status":"$([[ $phase == Succeeded || $phase == Running ]] && echo True || echo False)"}]},
 "version":1,"current_version":1}
JSON
)
        post PUT "$path/$name/status" "$status" || echo "status failed for $g/$ns/$k/$name" >&2
      fi
    fi
  done
done
echo "seed: created $created resources in $SM"
