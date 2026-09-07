#!/usr/bin/env python3
"""
Update spreadsheet cells with HYPERLINK formulas.

Usage:
    python3 update_sheet.py <year> <cell> <value> [file_id]
    python3 update_sheet.py 2023 F111 "1098" 1U0JzskcMSFQReclxIE7G_fVmcp0EfK0Z

If file_id is provided, creates a HYPERLINK formula.
If file_id is omitted, writes plain value.
"""

import sys
import os
import re

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../data-access/scripts'))
from connect import get_worksheet, get_credentials, SPREADSHEET_ID
from googleapiclient.discovery import build


def make_hyperlink(file_id: str, display_value: str) -> str:
    """Create Google Sheets HYPERLINK formula."""
    url = f"https://drive.google.com/file/d/{file_id}/view"
    # Escape quotes in display value
    escaped = display_value.replace('"', '""')
    return f'=HYPERLINK("{url}", "{escaped}")'


def parse_cell_address(cell: str) -> tuple:
    """Parse cell address like 'F152' into (col_index, row_index) 0-based."""
    match = re.match(r'^([A-Z]+)(\d+)$', cell.upper())
    if not match:
        raise ValueError(f"Invalid cell address: {cell}")
    col_str, row_str = match.groups()
    # Convert column letters to 0-based index (A=0, B=1, ..., Z=25, AA=26, etc.)
    col_index = 0
    for char in col_str:
        col_index = col_index * 26 + (ord(char) - ord('A') + 1)
    col_index -= 1  # Convert to 0-based
    row_index = int(row_str) - 1  # Convert to 0-based
    return col_index, row_index


def update_cell(year: str, cell: str, value: str, file_id: str = None):
    """
    Update a single cell in the spreadsheet.

    Args:
        year: Tab name (e.g., "2023")
        cell: Cell address (e.g., "F111")
        value: Display value
        file_id: Optional Google Drive file ID for hyperlink
    """
    worksheet = get_worksheet(year)
    sheet_id = worksheet.id

    if file_id:
        formula = make_hyperlink(file_id, value)
        # First update the formula
        worksheet.update(cell, [[formula]], value_input_option='USER_ENTERED')

        # Then set hyperlinkDisplayType to LINKED using Sheets API
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

        print(f"Updated {cell} with hyperlink to {value}")
    else:
        worksheet.update(cell, [[value]], value_input_option='USER_ENTERED')
        print(f"Updated {cell} with value: {value}")


def batch_update(year: str, updates: list):
    """
    Batch update multiple cells.

    Args:
        year: Tab name
        updates: List of {cell, value, file_id} dicts
    """
    worksheet = get_worksheet(year)
    sheet_id = worksheet.id
    hyperlink_cells = []

    for update in updates:
        cell = update['cell']
        value = update['value']
        file_id = update.get('file_id')

        if file_id:
            formula = make_hyperlink(file_id, value)
            worksheet.update(cell, [[formula]], value_input_option='USER_ENTERED')
            hyperlink_cells.append(cell)
        else:
            worksheet.update(cell, [[value]], value_input_option='USER_ENTERED')

    # Set hyperlinkDisplayType to LINKED for all hyperlink cells
    if hyperlink_cells:
        service = build('sheets', 'v4', credentials=get_credentials())
        requests = []
        for cell in hyperlink_cells:
            col_index, row_index = parse_cell_address(cell)
            requests.append({
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
            })
        service.spreadsheets().batchUpdate(
            spreadsheetId=SPREADSHEET_ID,
            body={'requests': requests}
        ).execute()

    print(f"Updated {len(updates)} cells")


def main():
    if len(sys.argv) < 4:
        print("Usage: python3 update_sheet.py <year> <cell> <value> [file_id]", file=sys.stderr)
        sys.exit(1)

    year = sys.argv[1]
    cell = sys.argv[2]
    value = sys.argv[3]
    file_id = sys.argv[4] if len(sys.argv) > 4 else None

    update_cell(year, cell, value, file_id)


if __name__ == '__main__':
    main()
