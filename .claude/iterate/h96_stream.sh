#!/bin/bash
# H96 aws-buckets -> cruzt USB, chunked ~100G, pigz -1, verify-on-cruzt BEFORE delete-on-H96.
# Laptop-orchestrated raw nc. Idempotent: lists built once; chunks whose source is gone are skipped.
set -uo pipefail
CRUZT=10.7.114.70
BASE=/home/travis/aws-buckets
DESTDIR=/mnt/seagate/HOMES/H96
PORT=9396
LL="$(dirname "$0")/h96_stream.log"
log(){ echo "[$(date -u +'%Y-%m-%d %H:%M:%S')] $*" >> "$LL"; }
: > "$LL"
log "START H96 stream-to-cruzt"

# Build the chunk lists on H96 ONCE (persisted), so resume reuses the same partitioning.
ssh H96 'sudo tee /var/tmp/h96_mklists.sh >/dev/null' <<'RS'
#!/bin/bash
set -uo pipefail
OUT=/var/tmp/h96-lists
BASE=/home/travis/aws-buckets
mkdir -p "$OUT"
if ls "$OUT"/.chunk.*.list >/dev/null 2>&1; then ls "$OUT"/.chunk.*.list | wc -l; exit 0; fi
cd "$BASE" || exit 2
find . -type f -printf '%s\t%p\n' | awk -v cb=$((100*1024*1024*1024)) -v out="$OUT" '
BEGIN{p=1;a=0}
{s=$1; sub(/^[0-9]+\t/,""); f=$0; if(a>0&&a+s>cb){p++;a=0}; print f >> (out"/.chunk."p".list"); a+=s}'
ls "$OUT"/.chunk.*.list 2>/dev/null | wc -l
RS
PARTS=$(ssh H96 'sudo bash /var/tmp/h96_mklists.sh' | tail -1)
log "planned parts=$PARTS"

n=0
while [ "$n" -lt "$PARTS" ]; do
  n=$((n+1))
  L="/var/tmp/h96-lists/.chunk.$n.list"
  TB="$DESTDIR/aws-part$n.tar.gz"
  # resume guard: if NONE of this chunk's source files remain, it already completed -> skip
  if ssh H96 "cd $BASE && sudo bash -c 'while IFS= read -r f; do [ -e \"\$f\" ] && exit 1; done < $L; exit 0'"; then
    log "PART $n already done (source gone) - skip"; PORT=$((PORT+1)); continue
  fi
  exp=$(ssh H96 "sudo wc -l < $L" | tr -d ' ')
  log "PART $n: $exp files -> $TB (port $PORT)"
  # listener on cruzt (writes the part), backgrounded locally
  ssh cruzt "sudo bash -c 'nc -l $PORT > $TB'" &
  CL=$!
  sleep 3
  if ssh H96 "cd $BASE && sudo bash -c 'set -o pipefail; tar --numeric-owner -cf - -T $L | pigz -1 | nc -N $CRUZT $PORT'"; then
    wait $CL 2>/dev/null
    if ssh cruzt "sudo pigz -t $TB"; then
      log "PART $n VERIFIED on cruzt ($(ssh cruzt "sudo du -h $TB|cut -f1")) -> deleting source on H96"
      ssh H96 "cd $BASE && sudo bash -c 'while IFS= read -r f; do rm -f \"\$f\"; done < $L'"
      log "PART $n source deleted; H96 free now $(ssh H96 "df -h / | awk \"NR==2{print \\\$4}\"")"
    else
      log "PART $n VERIFY FAILED on cruzt -> KEEP source, STOP"; exit 1
    fi
  else
    wait $CL 2>/dev/null
    log "PART $n SENDER pipeline FAILED -> KEEP source, STOP"; exit 1
  fi
  PORT=$((PORT+1))
done

ssh H96 "sudo find $BASE -type d -empty -delete 2>/dev/null; echo H96_remaining=\$(sudo du -sh $BASE 2>/dev/null|cut -f1)" >> "$LL" 2>&1
ssh cruzt "echo cruzt_H96_parts=\$(sudo ls $DESTDIR/aws-part*.tar.gz 2>/dev/null|wc -l) size=\$(sudo du -sh $DESTDIR 2>/dev/null|cut -f1)" >> "$LL" 2>&1
log "H96_STREAM_DONE"
