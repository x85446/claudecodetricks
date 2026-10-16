#!/usr/bin/env bash
set -euo pipefail

cmd_run() {
    local python="$1"; shift
    if [[ $# -gt 0 ]]; then
        exec "$python" combine_invoices.py "$@"
    fi
    if ! "$python" -c 'import pdfplumber, pypdf, reportlab' >/dev/null 2>&1; then
        echo "Error: PDF dependencies are missing. Run make install, then make run." >&2
        return 1
    fi
    local sample_dir
    sample_dir="$(mktemp -d "${TMPDIR:-/tmp}/tax-combiner-sample.XXXXXX")"
    trap 'rm -rf "$sample_dir"' EXIT
    "$python" - "$sample_dir" <<'PY'
import json, sys
from pathlib import Path
from reportlab.pdfgen import canvas
root = Path(sys.argv[1])
invoice = root / '2024 house Sample invoice.pdf'
pdf = canvas.Canvas(str(invoice))
pdf.drawString(72, 720, 'Sample maintenance invoice: $125.00')
pdf.save()
expenses = [{'file': str(invoice), 'date': '2024-01-02', 'vendor': 'Sample Vendor',
             'category': 'general', 'amount': 125, 'description': 'Sample maintenance'}]
(root / 'expenses.json').write_text(json.dumps(expenses))
PY
    "$python" combine_invoices.py --year 2024 --property Sample --input-dir "$sample_dir" \
        --expenses "$sample_dir/expenses.json" --output "$sample_dir/combined.pdf"
    "$python" - "$sample_dir/combined.pdf" <<'PY'
import sys
from pypdf import PdfReader
reader = PdfReader(sys.argv[1])
if len(reader.pages) != 2:
    raise SystemExit('Error: sample output must contain a summary and one invoice')
print('Sample complete: summary and invoice combined; disposable files removed on exit.')
PY
    rm -rf "$sample_dir"
    trap - EXIT
}

case "${1:-}" in
    run) shift; cmd_run "$@" ;;
    *) echo 'Usage: makehelp.sh run <python> [arguments...]' >&2; exit 2 ;;
esac
