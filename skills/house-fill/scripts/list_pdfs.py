#!/usr/bin/env python3
"""
List PDFs in Google Drive for a specific property and tax year.

Usage:
    python3 list_pdfs.py <property_address> <year>
    python3 list_pdfs.py "7207 Fence Line" 2023

Output:
    JSON array of {id, name, mimeType} for matching files
"""

import sys
import json
import os

# Add parent skills directory to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../data-access/scripts'))
from connect import get_drive, get_year_folder_id


def normalize_address(address: str) -> str:
    """Normalize property address for search."""
    # Remove spaces, convert to search-friendly format
    return address.replace(' ', '-').replace('_', '-')


def list_property_pdfs(property_address: str, year: str) -> list:
    """
    List all PDFs matching a property address in the year folder.

    Search pattern: "<year> houses <address>" in filename
    """
    drive = get_drive()
    folder_id = get_year_folder_id(year)

    # Normalize address for search
    search_terms = property_address.lower().split()

    # Build query - search for files containing property address keywords
    # Files are named like: "2023 houses 7207-Fence 1098.pdf"
    query = f"'{folder_id}' in parents and mimeType = 'application/pdf'"

    results = drive.files().list(
        q=query,
        spaces='drive',
        fields='files(id, name, mimeType, createdTime)',
        pageSize=100
    ).execute()

    files = results.get('files', [])

    # Filter by property address (case-insensitive partial match)
    matching = []
    for f in files:
        name_lower = f['name'].lower()
        # Check if file contains key address parts (e.g., "7207" and "fence")
        if all(term in name_lower for term in search_terms[:2]):  # Match first 2 terms
            matching.append({
                'id': f['id'],
                'name': f['name'],
                'mimeType': f['mimeType']
            })

    return matching


def main():
    if len(sys.argv) < 3:
        print("Usage: python3 list_pdfs.py <property_address> <year>", file=sys.stderr)
        print('Example: python3 list_pdfs.py "7207 Fence Line" 2023', file=sys.stderr)
        sys.exit(1)

    property_address = sys.argv[1]
    year = sys.argv[2]

    pdfs = list_property_pdfs(property_address, year)

    print(json.dumps(pdfs, indent=2))

    # Also print summary to stderr for human readability
    print(f"\nFound {len(pdfs)} PDFs for {property_address} ({year}):", file=sys.stderr)
    for pdf in pdfs:
        print(f"  - {pdf['name']}", file=sys.stderr)


if __name__ == '__main__':
    main()
