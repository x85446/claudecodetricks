---
name: production-process-ai
description: Production workflow for processing finance files from the incoming folder using the renaming-finance-files skill.
---

# Production Finance File Processing

Process finance files from the production incoming folder using the `renaming-finance-files` skill.

## Workflow

1. **Check for primary directory** (macOS/OneDrive):
   - Source: `/Users/travis/Library/CloudStorage/OneDrive-izumanet/Finance/Invoice_processing/incoming`
   - Destination: `/Users/travis/Library/CloudStorage/OneDrive-izumanet/Finance/Invoice_processing/incoming-temp`

2. **If primary doesn't exist, use fallback** (workspace):
   - Source: `/workspace/processing/incoming`
   - Destination: `/workspace/processing/incoming-temp`

3. **Invoke the renaming-finance-files skill** with the source directory

4. **Files are organized as follows:**
   - Finance documents (invoices, statements, receipts) → renamed and moved to destination
   - Authorization forms → moved to `<base>/authorizations-temp/` (original filename)
   - Service orders/contracts → moved to `<base>/contracts-temp/` (original filename)
   - Other forms/questionnaires → moved to `<base>/forms-temp/` (original filename)

   Where `<base>` is the parent directory of the source (e.g., `/workspace/processing/` or the OneDrive `Invoice_processing/` folder). Non-finance documents go to sibling `-temp` directories, not subdirectories of the destination.

## Invocation

```
/production-process-ai
```

Or: "Run production finance processing"
