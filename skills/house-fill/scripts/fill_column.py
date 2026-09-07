#!/usr/bin/env python3
"""
Utility functions for filling house data. Claude orchestrates the workflow.

This script provides helpers - Claude reads the spreadsheet, discovers
the structure, and decides what to write where.
"""

import sys
import os
import re
import json

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../data-access/scripts'))
from connect import get_worksheet, get_credentials, SPREADSHEET_ID
from googleapiclient.discovery import build

from list_pdfs import list_property_pdfs
from download_pdf import extract_text


def parse_cell_address(cell: str) -> tuple:
    """Parse cell address like 'F152' into (col_index, row_index) 0-based."""
    match = re.match(r'^([A-Z]+)(\d+)$', cell.upper())
    if not match:
        raise ValueError(f"Invalid cell address: {cell}")
    col_str, row_str = match.groups()
    col_index = 0
    for char in col_str:
        col_index = col_index * 26 + (ord(char) - ord('A') + 1)
    col_index -= 1
    row_index = int(row_str) - 1
    return col_index, row_index


def read_section(year: str, start_row: int, end_row: int, start_col: str = 'A', end_col: str = 'L') -> list:
    """
    Read a section of the spreadsheet.
    Returns list of rows, each row is a list of cell values.
    """
    worksheet = get_worksheet(year)
    range_str = f"{start_col}{start_row}:{end_col}{end_row}"
    data = worksheet.get(range_str)

    print(f"Read {len(data)} rows from {year}!{range_str}")
    for i, row in enumerate(data):
        print(f"  Row {start_row + i}: {row}")

    return data


def find_in_sheet(year: str, search_text: str, start_row: int = 1, end_row: int = 200) -> list:
    """
    Find all cells containing search_text.
    Returns list of {row, col, value} dicts.
    """
    worksheet = get_worksheet(year)

    # Get all values in range
    data = worksheet.get(f"A{start_row}:Z{end_row}")

    results = []
    search_lower = search_text.lower()

    for row_idx, row in enumerate(data):
        for col_idx, cell in enumerate(row):
            if cell and search_lower in str(cell).lower():
                col_letter = chr(ord('A') + col_idx) if col_idx < 26 else None
                results.append({
                    'row': start_row + row_idx,
                    'col': col_letter,
                    'col_index': col_idx,
                    'value': cell
                })

    return results


def write_cell(year: str, cell: str, value: str, as_hyperlink: bool = False, file_id: str = None):
    """
    Write a value to a cell.
    If as_hyperlink=True and file_id provided, creates HYPERLINK formula
    and sets hyperlinkDisplayType to LINKED so it appears as a clickable link.
    """
    worksheet = get_worksheet(year)
    sheet_id = worksheet.id

    if as_hyperlink and file_id:
        url = f"https://drive.google.com/file/d/{file_id}/view"
        escaped = str(value).replace('"', '""')
        formula = f'=HYPERLINK("{url}", "{escaped}")'
        worksheet.update(cell, [[formula]], value_input_option='USER_ENTERED')

        # Set hyperlinkDisplayType to LINKED so the link is clickable
        col_index, row_index = parse_cell_address(cell)
        service = build('sheets', 'v4', credentials=get_credentials())
        request = {
            'requests': [{
                'repeatCell': {
                    'range': {
                        'sheetId': sheet_id,
                        'startRowIndex': row_index,
                        'endRowIndex': row_index + 1,
                        'startColumnIndex': col_index,
                        'endColumnIndex': col_index + 1
                    },
                    'cell': {
                        'userEnteredFormat': {
                            'hyperlinkDisplayType': 'LINKED'
                        }
                    },
                    'fields': 'userEnteredFormat.hyperlinkDisplayType'
                }
            }]
        }
        service.spreadsheets().batchUpdate(
            spreadsheetId=SPREADSHEET_ID,
            body=request
        ).execute()

        print(f"Wrote hyperlink to {cell}: {value}")
    else:
        worksheet.update(cell, [[value]], value_input_option='USER_ENTERED')
        print(f"Wrote to {cell}: {value}")


def parse_currency(text: str) -> float:
    """Extract currency amount from text."""
    match = re.search(r'\$?([\d,]+\.?\d*)', text.replace(',', ''))
    if match:
        return float(match.group(1).replace(',', ''))
    return None


def add_note(year: str, cell: str, note: str):
    """
    Add a note (comment) to a cell without changing the cell value.
    """
    worksheet = get_worksheet(year)
    sheet_id = worksheet.id
    col_index, row_index = parse_cell_address(cell)

    service = build('sheets', 'v4', credentials=get_credentials())
    request = {
        'requests': [{
            'updateCells': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': row_index,
                    'endRowIndex': row_index + 1,
                    'startColumnIndex': col_index,
                    'endColumnIndex': col_index + 1
                },
                'rows': [{
                    'values': [{
                        'note': note
                    }]
                }],
                'fields': 'note'
            }
        }]
    }
    service.spreadsheets().batchUpdate(
        spreadsheetId=SPREADSHEET_ID,
        body=request
    ).execute()

    print(f"Added note to {cell}: {note}")


def main():
    """Interactive mode - show usage."""
    print("""
House Fill Utilities
====================

Functions available (import in Python or call via Claude):

1. read_section(year, start_row, end_row, start_col, end_col)
   - Read a range from the spreadsheet

2. find_in_sheet(year, search_text, start_row, end_row)
   - Find cells containing text, returns row/col positions

3. write_cell(year, cell, value, as_hyperlink, file_id)
   - Write value to cell, optionally as hyperlink

4. list_property_pdfs(property_address, year)
   - List PDFs matching property in Drive folder

5. extract_text(file_id)
   - Download PDF and extract text

Example workflow:
    # Find where "7207 Fence Line" appears
    results = find_in_sheet("2023", "7207 Fence Line")

    # Read the houses section
    data = read_section("2023", 97, 160, "E", "K")

    # Write a value with hyperlink
    write_cell("2023", "F111", "1098", as_hyperlink=True, file_id="abc123")
""")


if __name__ == '__main__':
    main()
