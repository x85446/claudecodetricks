"""Shared helpers for normalizers/*.py. Not a source module (leading "_" ->
skipped by normalizers.discover())."""
import datetime
import html
import re


def clean(s):
    if not isinstance(s, str):
        return s
    s = re.sub(r'<[^>]+>', ' ', s)
    s = html.unescape(s)
    s = re.sub(r'\s+', ' ', s).strip()
    return s


def iso(d):
    """Normalize a date to ISO YYYY-MM-DD. Handles ISO strings, epoch (ms/s),
    and US formats Grants.gov sometimes emits ('Jul 10, 2026', '07/10/2026')."""
    if not d:
        return None
    if isinstance(d, (int, float)):
        secs = d / 1000.0 if d > 1e11 else float(d)
        try:
            return datetime.datetime.utcfromtimestamp(secs).strftime('%Y-%m-%d')
        except Exception:
            return None
    if not isinstance(d, str):
        return None
    s = d.strip()
    # Already ISO-ish (YYYY-MM-DD...) — take the date head.
    if re.match(r'\d{4}-\d{2}-\d{2}', s):
        return s[:10]
    for fmt in ('%b %d, %Y', '%B %d, %Y', '%b %d %Y', '%B %d %Y',
                '%m/%d/%Y', '%m/%d/%y', '%Y/%m/%d'):
        try:
            return datetime.datetime.strptime(s.replace('.', '').strip(), fmt).strftime('%Y-%m-%d')
        except Exception:
            continue
    return None  # unparseable -> null (better than a corrupted prefix)


def epoch_or_iso(v):
    """DSIP dates may be epoch-millis ints, epoch-seconds, or ISO strings."""
    if v is None:
        return None
    if isinstance(v, (int, float)):
        secs = v / 1000.0 if v > 1e11 else float(v)
        try:
            return datetime.datetime.utcfromtimestamp(secs).strftime('%Y-%m-%d')
        except Exception:
            return None
    if isinstance(v, str):
        return v[:10] if v else None
    return None


def num(v):
    """funding_ceiling is a float in the unified schema; Grants.gov sends it as a
    string on some records and 0 to mean 'unstated'."""
    try:
        f = float(str(v).replace(',', '').replace('$', '').strip())
    except Exception:
        return None
    return f or None


def mmddyyyy(s):
    """Grants.gov bulk-extract dates are MMDDYYYY strings -> ISO YYYY-MM-DD."""
    if not isinstance(s, str):
        return None
    s = s.strip()
    if not re.fullmatch(r'\d{8}', s):
        return None
    try:
        return datetime.datetime.strptime(s, '%m%d%Y').strftime('%Y-%m-%d')
    except Exception:
        return None


# Grants.gov EligibleApplicants numeric codes -> human label (for eligibility text).
ELIG_CODE = {
    '00': 'State governments', '01': 'County governments', '02': 'City/township governments',
    '04': 'Special district governments', '05': 'Independent school districts',
    '06': 'Public/State institutions of higher ed', '07': 'Federally recognized tribal governments',
    '08': 'Public/Indian housing authorities', '11': 'Native American tribal organizations',
    '12': 'Nonprofits 501(c)(3)', '13': 'Nonprofits non-501(c)(3)',
    '20': 'Private institutions of higher ed', '21': 'Individuals',
    '22': 'For-profit (other than small business)', '23': 'Small businesses',
    '25': 'Others', '99': 'Unrestricted',
}
