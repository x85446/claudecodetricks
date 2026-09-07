#!/usr/bin/env python3
"""
Download a PDF from Google Drive and extract its text.

Usage:
    python3 download_pdf.py <file_id>
    python3 download_pdf.py 1U0JzskcMSFQReclxIE7G_fVmcp0EfK0Z

Output:
    Extracted text from all pages of the PDF
"""

import sys
import io
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../data-access/scripts'))
from connect import get_drive

from googleapiclient.http import MediaIoBaseDownload
import pdfplumber


def download_pdf_bytes(file_id: str) -> io.BytesIO:
    """Download PDF from Google Drive as BytesIO."""
    drive = get_drive()
    request = drive.files().get_media(fileId=file_id)

    fh = io.BytesIO()
    downloader = MediaIoBaseDownload(fh, request)

    done = False
    while not done:
        status, done = downloader.next_chunk()

    fh.seek(0)
    return fh


def extract_text(file_id: str) -> str:
    """Download PDF and extract all text."""
    pdf_bytes = download_pdf_bytes(file_id)

    text_parts = []
    with pdfplumber.open(pdf_bytes) as pdf:
        for i, page in enumerate(pdf.pages):
            page_text = page.extract_text() or ""
            if page_text:
                text_parts.append(f"=== Page {i+1} ===\n{page_text}")

    return "\n\n".join(text_parts)


def extract_tables(file_id: str) -> list:
    """Download PDF and extract tables (useful for 1098 forms)."""
    pdf_bytes = download_pdf_bytes(file_id)

    tables = []
    with pdfplumber.open(pdf_bytes) as pdf:
        for i, page in enumerate(pdf.pages):
            page_tables = page.extract_tables()
            for table in page_tables:
                tables.append({
                    'page': i + 1,
                    'data': table
                })

    return tables


def main():
    if len(sys.argv) < 2:
        print("Usage: python3 download_pdf.py <file_id>", file=sys.stderr)
        sys.exit(1)

    file_id = sys.argv[1]
    text = extract_text(file_id)
    print(text)


if __name__ == '__main__':
    main()
