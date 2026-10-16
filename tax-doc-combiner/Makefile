# Tax Document Combiner - Makefile
# Manages dependencies for PDF manipulation

.DEFAULT_GOAL := help
.PHONY: all build run install check clean help test

# Python executable
PYTHON := python3
PIP := pip3

# Required packages
PACKAGES := pdfplumber pypdf reportlab

##@ General
help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"; print "Tax Document Combiner"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-18s %s\n", $$1, $$2} /^##@/ {print "\n" substr($$0, 5)}' $(MAKEFILE_LIST)

##@ Build
all: build ## Build the combiner

build: ## Check the combiner's Python syntax
	@$(PYTHON) -c "import ast,pathlib; ast.parse(pathlib.Path('combine_invoices.py').read_text()); print('Combiner ready')"

##@ Install
install: ## Install PDF dependencies
	@echo "Installing required packages..."
	$(PIP) install $(PACKAGES)
	@echo ""
	@echo "Installation complete. Verifying..."
	@$(MAKE) check

##@ Quality
check: ## Report installed PDF dependencies
	@echo "Checking dependencies..."
	@$(PYTHON) -c "import pdfplumber; print('  pdfplumber:', pdfplumber.__version__)" 2>/dev/null || echo "  pdfplumber: NOT INSTALLED"
	@$(PYTHON) -c "import pypdf; print('  pypdf:', pypdf.__version__)" 2>/dev/null || echo "  pypdf: NOT INSTALLED"
	@$(PYTHON) -c "import reportlab; print('  reportlab:', reportlab.Version)" 2>/dev/null || echo "  reportlab: NOT INSTALLED"
	@echo ""
	@$(PYTHON) -c "import pdfplumber, pypdf, reportlab" 2>/dev/null && echo "All dependencies OK" || echo "Some dependencies missing - run 'make install'"

##@ Cleanup
clean: ## Remove generated Python cache files
	@echo "Cleaning generated files..."
	find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@echo "Clean complete."

# Test the combiner with a dry run
##@ Test
test: ## Exercise argument help
	@echo "Running script with --help..."
	$(PYTHON) combine_invoices.py --help

##@ Development
run: build ## Combine disposable sample invoices (ARGS passes CLI arguments)
	@./makehelp.sh run "$(PYTHON)" $(ARGS)
