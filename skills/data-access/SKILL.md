---
name: data-access
description: Access the T&M Master spreadsheet and Google Drive folders containing source PDFs.
allowed-tools: Read,Bash(python3:*)
---

# Data Access

Provides authentication and connectivity to T&M Master spreadsheet and Google Drive.

## Credentials
- **Service Account**: `mcp-493@weighty-wonder-475019-f7.iam.gserviceaccount.com`
- **Credentials File**: `~/.ssh/google-tmctech-mcp.json`

## Spreadsheet
- **Name**: T&M MASTER
- **ID**: `1mBczdUzgkT2ZTXQr9jVY8L7wkE0cQm0RQyFtF9Qi8fs`
- **Tabs**: Year-based (2023, 2024, etc.)

## Google Drive
- **Root URL**: https://drive.google.com/drive/u/0/folders/1IQLWsqriCawKovX1wxSSmjI09SkVUch3
- **Root Folder ID**: `1IQLWsqriCawKovX1wxSSmjI09SkVUch3`

### Tax Year Folders
| Year | Folder ID |
|------|-----------|
| 2023 | `1-vzOnkuE_cyaCMPSSdisplSMGlTHdYty` |
| 2024 | `147r5yvPiTqQcvo51QAEa1DUWiO5uhSpE` |

## Scripts

```bash
# Test connection
python3 {baseDir}/scripts/connect.py

# Get authenticated clients (in Python)
from connect import get_drive, get_sheets, get_worksheet
```

## Usage in Other Skills

Import the connect module:
```python
import sys, os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../data-access/scripts'))
from connect import get_drive, get_sheets, get_worksheet, get_year_folder_id
```
