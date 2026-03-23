#!/usr/bin/env python3
"""
Combine house maintenance invoices into a master invoice with summary table.

Usage:
    python3 combine_invoices.py --year 2024 --property "1234-Main" \
        --input-dir "2024/" --output "2024/2024 houses 1234-Main master-invoice.pdf"

Or with explicit expense data (JSON from Claude):
    python3 combine_invoices.py --expenses expenses.json \
        --invoices invoice1.pdf invoice2.pdf \
        --output master-invoice.pdf
"""

import argparse
import json
import glob
import os
import sys
from datetime import datetime
from io import BytesIO
from pathlib import Path

try:
    import pdfplumber
    from reportlab.lib import colors
    from reportlab.lib.pagesizes import letter
    from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
    from reportlab.lib.units import inch
    from reportlab.platypus import SimpleDocTemplate, Table, TableStyle, Paragraph, Spacer
    from pypdf import PdfReader, PdfWriter
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("Run: make -C .claude/skills/tax-doc-combiner install")
    sys.exit(1)


CATEGORIES = {
    'plumbing': ['plumber', 'plumbing', 'drain', 'pipe', 'faucet', 'water heater',
                 'toilet', 'leak', 'sewer', 'water line'],
    'electrical': ['electrician', 'electrical', 'wiring', 'outlet', 'panel',
                   'breaker', 'lighting', 'switch', 'circuit'],
    'hvac': ['heating', 'cooling', 'ac ', 'a/c', 'furnace', 'duct', 'thermostat',
             'hvac', 'air conditioning', 'heat pump', 'condenser'],
    'yard': ['landscaping', 'landscape', 'lawn', 'tree', 'irrigation', 'sprinkler',
             'fence', 'mowing', 'garden', 'yard', 'mulch', 'trimming'],
    'roofing': ['roof', 'roofing', 'shingle', 'gutter', 'flashing'],
    'appliance': ['appliance', 'refrigerator', 'dishwasher', 'washer', 'dryer',
                  'oven', 'microwave', 'disposal', 'range'],
    'flooring': ['floor', 'flooring', 'carpet', 'tile', 'hardwood', 'laminate', 'vinyl'],
    'painting': ['paint', 'painter', 'painting', 'stain', 'primer'],
    'pest': ['pest', 'termite', 'exterminator', 'rodent', 'insect', 'bug'],
    'cleaning': ['cleaning', 'clean', 'maid', 'janitorial', 'pressure wash'],
    'general': ['handyman', 'repair', 'maintenance', 'misc', 'service'],
}


def categorize_expense(text: str) -> str:
    """Determine expense category from invoice text."""
    text_lower = text.lower()

    # Check each category (except general) for matches
    for category, keywords in CATEGORIES.items():
        if category == 'general':
            continue
        for keyword in keywords:
            if keyword in text_lower:
                return category

    return 'general'


def parse_amount(text: str) -> float | None:
    """Extract dollar amount from text."""
    import re
    # Look for dollar amounts like $1,234.56 or 1234.56
    patterns = [
        r'\$\s*([\d,]+\.?\d*)',  # $1,234.56
        r'(?:total|amount|due|balance)[:\s]*\$?\s*([\d,]+\.?\d*)',  # Total: 1234.56
    ]

    for pattern in patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        if matches:
            # Return the largest amount found (likely the total)
            amounts = []
            for m in matches:
                try:
                    amounts.append(float(m.replace(',', '')))
                except ValueError:
                    continue
            if amounts:
                return max(amounts)
    return None


def parse_date(text: str) -> str | None:
    """Extract date from invoice text."""
    import re

    # Common date patterns
    patterns = [
        (r'(\d{1,2})[/\-](\d{1,2})[/\-](\d{4})', '%m/%d/%Y'),  # MM/DD/YYYY
        (r'(\d{4})[/\-](\d{1,2})[/\-](\d{1,2})', '%Y-%m-%d'),  # YYYY-MM-DD
        (r'([A-Za-z]+)\s+(\d{1,2}),?\s+(\d{4})', '%B %d %Y'),  # Month DD, YYYY
    ]

    for pattern, fmt in patterns:
        match = re.search(pattern, text)
        if match:
            try:
                if fmt == '%m/%d/%Y':
                    return f"{match.group(3)}-{match.group(1).zfill(2)}-{match.group(2).zfill(2)}"
                elif fmt == '%Y-%m-%d':
                    return f"{match.group(1)}-{match.group(2).zfill(2)}-{match.group(3).zfill(2)}"
                else:
                    dt = datetime.strptime(match.group(0).replace(',', ''), fmt)
                    return dt.strftime('%Y-%m-%d')
            except ValueError:
                continue
    return None


def extract_vendor(text: str, filename: str) -> str:
    """Try to extract vendor name from invoice."""
    # Often the vendor is at the top of the invoice
    lines = text.split('\n')
    for line in lines[:10]:  # Check first 10 lines
        line = line.strip()
        # Skip empty lines, dates, amounts, addresses
        if not line or len(line) < 3:
            continue
        if line.startswith('$') or line[0].isdigit():
            continue
        if any(word in line.lower() for word in ['invoice', 'bill', 'statement', 'date', 'total']):
            continue
        # This might be the vendor name
        if len(line) > 3 and len(line) < 50:
            return line

    # Fallback: use filename
    return Path(filename).stem


def read_invoice_pdf(filepath: str) -> dict:
    """Extract invoice data from PDF."""
    with pdfplumber.open(filepath) as pdf:
        text = ""
        for page in pdf.pages:
            text += page.extract_text() or ""

    return {
        'file': filepath,
        'text': text,
        'date': parse_date(text),
        'amount': parse_amount(text),
        'vendor': extract_vendor(text, filepath),
        'category': categorize_expense(text),
    }


def create_summary_pdf(expenses: list[dict], year: str, property_id: str) -> bytes:
    """Create a PDF with the expense summary table."""
    buffer = BytesIO()
    doc = SimpleDocTemplate(buffer, pagesize=letter,
                           topMargin=0.75*inch, bottomMargin=0.75*inch,
                           leftMargin=0.5*inch, rightMargin=0.5*inch)

    styles = getSampleStyleSheet()
    title_style = ParagraphStyle(
        'CustomTitle',
        parent=styles['Heading1'],
        fontSize=18,
        spaceAfter=20,
        alignment=1,  # Center
    )

    elements = []

    # Title
    elements.append(Paragraph(f"PROPERTY EXPENSE SUMMARY", title_style))
    elements.append(Paragraph(f"{year} - {property_id}", styles['Heading2']))
    elements.append(Spacer(1, 20))

    # Main expense table
    table_data = [['Date', 'Vendor', 'Category', 'Description', 'Amount']]

    total = 0.0
    category_totals = {}

    for exp in sorted(expenses, key=lambda x: x.get('date') or ''):
        date = exp.get('date') or 'Unknown'
        vendor = exp.get('vendor') or 'Unknown'
        category = exp.get('category') or 'general'
        description = exp.get('description') or ''
        amount = exp.get('amount') or 0.0

        # Truncate long fields
        vendor = vendor[:25] + '...' if len(vendor) > 28 else vendor
        description = description[:20] + '...' if len(description) > 23 else description

        table_data.append([
            date,
            vendor,
            category,
            description,
            f"${amount:,.2f}"
        ])

        total += amount
        category_totals[category] = category_totals.get(category, 0) + amount

    # Add total row
    table_data.append(['', '', '', 'TOTAL:', f"${total:,.2f}"])

    # Create and style the table
    col_widths = [0.9*inch, 1.8*inch, 0.9*inch, 1.5*inch, 0.9*inch]
    table = Table(table_data, colWidths=col_widths)
    table.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), colors.grey),
        ('TEXTCOLOR', (0, 0), (-1, 0), colors.whitesmoke),
        ('ALIGN', (0, 0), (-1, -1), 'LEFT'),
        ('ALIGN', (-1, 0), (-1, -1), 'RIGHT'),  # Right-align amounts
        ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
        ('FONTSIZE', (0, 0), (-1, -1), 9),
        ('BOTTOMPADDING', (0, 0), (-1, 0), 12),
        ('BACKGROUND', (0, -1), (-1, -1), colors.lightgrey),
        ('FONTNAME', (0, -1), (-1, -1), 'Helvetica-Bold'),
        ('GRID', (0, 0), (-1, -1), 1, colors.black),
    ]))

    elements.append(table)
    elements.append(Spacer(1, 30))

    # Category totals table
    elements.append(Paragraph("CATEGORY TOTALS", styles['Heading3']))
    elements.append(Spacer(1, 10))

    cat_data = [['Category', 'Total']]
    for cat in sorted(category_totals.keys()):
        cat_data.append([cat, f"${category_totals[cat]:,.2f}"])

    cat_table = Table(cat_data, colWidths=[2*inch, 1.5*inch])
    cat_table.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), colors.grey),
        ('TEXTCOLOR', (0, 0), (-1, 0), colors.whitesmoke),
        ('ALIGN', (0, 0), (0, -1), 'LEFT'),
        ('ALIGN', (1, 0), (1, -1), 'RIGHT'),
        ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
        ('FONTSIZE', (0, 0), (-1, -1), 10),
        ('GRID', (0, 0), (-1, -1), 1, colors.black),
    ]))

    elements.append(cat_table)
    elements.append(Spacer(1, 20))

    # Generation timestamp
    elements.append(Paragraph(
        f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M')}",
        styles['Normal']
    ))

    doc.build(elements)
    return buffer.getvalue()


def combine_pdfs(summary_pdf: bytes, invoice_files: list[str], output_path: str):
    """Combine summary PDF with all invoice PDFs."""
    writer = PdfWriter()

    # Add summary page first
    summary_reader = PdfReader(BytesIO(summary_pdf))
    for page in summary_reader.pages:
        writer.add_page(page)

    # Add each invoice
    for invoice_file in invoice_files:
        reader = PdfReader(invoice_file)
        for page in reader.pages:
            writer.add_page(page)

    # Write output
    with open(output_path, 'wb') as f:
        writer.write(f)


def main():
    parser = argparse.ArgumentParser(description='Combine house invoices into master invoice')
    parser.add_argument('--year', required=True, help='Tax year (e.g., 2024)')
    parser.add_argument('--property', required=True, help='Property ID (e.g., 1234-Main)')
    parser.add_argument('--input-dir', required=True, help='Directory containing invoices')
    parser.add_argument('--output', required=True, help='Output PDF path')
    parser.add_argument('--expenses', help='JSON file with expense data (optional, will auto-extract if not provided)')
    parser.add_argument('--invoices', nargs='*', help='Specific invoice files (optional)')

    args = parser.parse_args()

    # Find invoice files
    if args.invoices:
        invoice_files = args.invoices
    else:
        pattern = os.path.join(args.input_dir, f"{args.year} house {args.property} invoice*.pdf")
        invoice_files = sorted(glob.glob(pattern, recursive=False))

        if not invoice_files:
            # Try case-insensitive search
            pattern = os.path.join(args.input_dir, f"*house*{args.property}*invoice*.pdf")
            invoice_files = [f for f in glob.glob(pattern) if args.year in f]

    if not invoice_files:
        print(f"No invoices found matching pattern in {args.input_dir}")
        sys.exit(1)

    print(f"Found {len(invoice_files)} invoice(s):")
    for f in invoice_files:
        print(f"  - {f}")

    # Extract or load expense data
    if args.expenses:
        with open(args.expenses) as f:
            expenses = json.load(f)
    else:
        print("\nExtracting expense data...")
        expenses = []
        for invoice_file in invoice_files:
            print(f"  Reading: {os.path.basename(invoice_file)}")
            data = read_invoice_pdf(invoice_file)
            expenses.append({
                'file': invoice_file,
                'date': data['date'],
                'vendor': data['vendor'],
                'category': data['category'],
                'amount': data['amount'],
                'description': '',  # Would need more sophisticated extraction
            })
            print(f"    Date: {data['date']}, Vendor: {data['vendor']}, "
                  f"Category: {data['category']}, Amount: ${data['amount'] or 0:.2f}")

    # Validate we have amounts
    missing_amounts = [e for e in expenses if not e.get('amount')]
    if missing_amounts:
        print("\nWARNING: Could not extract amounts from:")
        for e in missing_amounts:
            print(f"  - {e.get('file', 'Unknown')}")
        print("Please provide amounts manually or check the invoices.")

    # Create summary PDF
    print("\nGenerating summary page...")
    summary_pdf = create_summary_pdf(expenses, args.year, args.property)

    # Combine PDFs
    print(f"Combining into: {args.output}")
    combine_pdfs(summary_pdf, invoice_files, args.output)

    # Print summary
    total = sum(e.get('amount') or 0 for e in expenses)
    print(f"\nSummary:")
    print(f"  Invoices combined: {len(invoice_files)}")
    print(f"  Total expenses: ${total:,.2f}")
    print(f"  Output: {args.output}")

    # Output JSON for Claude to use
    print("\n--- EXPENSE_DATA_JSON ---")
    print(json.dumps(expenses, indent=2))
    print("--- END_EXPENSE_DATA_JSON ---")


if __name__ == '__main__':
    main()
