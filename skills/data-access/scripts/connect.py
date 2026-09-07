#!/usr/bin/env python3
"""
Shared authentication module for T&M Master spreadsheet and Google Drive.

Usage:
    from connect import get_drive, get_sheets, get_worksheet

Returns authenticated clients for Drive API and gspread.
"""

import os
from google.oauth2.service_account import Credentials
from googleapiclient.discovery import build
import gspread

# Configuration
CREDENTIALS_PATH = os.path.expanduser('~/.ssh/google-tmctech-mcp.json')
SPREADSHEET_ID = '1mBczdUzgkT2ZTXQr9jVY8L7wkE0cQm0RQyFtF9Qi8fs'
SCOPES = [
    'https://www.googleapis.com/auth/spreadsheets',
    'https://www.googleapis.com/auth/drive.readonly'
]

# Tax year folder IDs
YEAR_FOLDERS = {
    '2023': '1-vzOnkuE_cyaCMPSSdisplSMGlTHdYty',
    '2024': '147r5yvPiTqQcvo51QAEa1DUWiO5uhSpE',
}

_credentials = None
_drive_client = None
_sheets_client = None


def get_credentials():
    """Get cached credentials."""
    global _credentials
    if _credentials is None:
        _credentials = Credentials.from_service_account_file(
            CREDENTIALS_PATH, scopes=SCOPES
        )
    return _credentials


def get_drive():
    """Get authenticated Google Drive API client."""
    global _drive_client
    if _drive_client is None:
        _drive_client = build('drive', 'v3', credentials=get_credentials())
    return _drive_client


def get_sheets():
    """Get authenticated gspread client."""
    global _sheets_client
    if _sheets_client is None:
        _sheets_client = gspread.authorize(get_credentials())
    return _sheets_client


def get_spreadsheet():
    """Get T&M Master spreadsheet."""
    return get_sheets().open_by_key(SPREADSHEET_ID)


def get_worksheet(year: str):
    """Get worksheet for a specific tax year."""
    return get_spreadsheet().worksheet(year)


def get_year_folder_id(year: str) -> str:
    """Get Google Drive folder ID for a tax year."""
    if year not in YEAR_FOLDERS:
        raise ValueError(f"Unknown year: {year}. Known years: {list(YEAR_FOLDERS.keys())}")
    return YEAR_FOLDERS[year]


if __name__ == '__main__':
    # Test connection
    print("Testing connections...")
    drive = get_drive()
    print(f"  Drive API: OK")
    sheets = get_sheets()
    print(f"  Sheets API: OK")
    spreadsheet = get_spreadsheet()
    print(f"  Spreadsheet: {spreadsheet.title}")
    print(f"  Tabs: {[ws.title for ws in spreadsheet.worksheets()]}")
