# Tax Document Combiner - Makefile
# Manages dependencies for PDF manipulation

.PHONY: install check clean help

# Python executable
PYTHON := python3
PIP := pip3

# Required packages
PACKAGES := pdfplumber pypdf reportlab

help:
	@echo "Tax Document Combiner - Dependency Management"
	@echo ""
	@echo "Usage:"
	@echo "  make install    Install required Python packages"
	@echo "  make check      Check if dependencies are installed"
	@echo "  make clean      Remove generated files"
	@echo "  make help       Show this help message"
	@echo ""
	@echo "Required packages: $(PACKAGES)"

install:
	@echo "Installing required packages..."
	$(PIP) install $(PACKAGES)
	@echo ""
	@echo "Installation complete. Verifying..."
	@$(MAKE) check

check:
	@echo "Checking dependencies..."
	@$(PYTHON) -c "import pdfplumber; print('  pdfplumber:', pdfplumber.__version__)" 2>/dev/null || echo "  pdfplumber: NOT INSTALLED"
	@$(PYTHON) -c "import pypdf; print('  pypdf:', pypdf.__version__)" 2>/dev/null || echo "  pypdf: NOT INSTALLED"
	@$(PYTHON) -c "import reportlab; print('  reportlab:', reportlab.Version)" 2>/dev/null || echo "  reportlab: NOT INSTALLED"
	@echo ""
	@$(PYTHON) -c "import pdfplumber, pypdf, reportlab" 2>/dev/null && echo "All dependencies OK" || echo "Some dependencies missing - run 'make install'"

clean:
	@echo "Cleaning generated files..."
	find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@echo "Clean complete."

# Test the combiner with a dry run
test:
	@echo "Running script with --help..."
	$(PYTHON) combine_invoices.py --help
