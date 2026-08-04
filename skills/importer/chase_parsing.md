# Chase parsing reference

Supporting notes for the Chase section of the importer skill. Everything
here is about the shapes Chase emits; the rules live in SKILL.md.

## Credit-card PDF (9878) — raw text sample

`PyMuPDF.page.get_text()` on page 2 of `230407-chase-9878.pdf` yields lines
like:

```
'03/15     '
'UPGRADED BOARDING CREDIT'
'-40.00'
'03/07     '
'HAT CREEK BURGERS-WESTLAK WEST LAKE HIL TX'
'36.55'
'03/09     '
'SOUTH AUSTIN MEDICAL CLIN AUSTIN TX'
'10.00'
'03/11     '
'MED*AUSTIN SPORTS MED 512-450-1300 TX'
'10.00'
```

So descriptions frequently include:
- Merchant name + city + 2-letter state.
- Phone numbers (10 digits, often hyphenated) in place of city.
- Store numbers (`#01136`, `#02774*`, `7-ELEVEN 36565`).
- URL-ish fragments like `HTTPSGITHUB.C CA`, `WWW.TEAMUNIFY TX`.
- `*` and `&` delimiters inside merchant names (`TST* Michelinos Cafe Ole`,
  `WESTLAKE AQUATIC & WOW`).

Trailing-state stripper must treat `WESTLAKE AQUATIC & WOW WWW.TEAMUNIFY TX`
as a merchant whose URL ends in `.C` — the `CITY ST` pattern applies, and
`WWW.TEAMUNIFY` (the token before `TX`) has non-alpha characters so the
strip fires. Result: `WESTLAKE AQUATIC & WOW`. Contrast with
`TST* Michelinos Cafe Ole San Antonio TX` — the token before `TX` is `Antonio`
(pure alpha), so the strip must NOT fire; multi-word cities stay in place.

## Banking PDF (9956 / 8073) — raw text sample

The checking/savings statements put descriptions on their own lines
between the date and the amount. One transaction can span 2–4 lines:

```
'04/28'
'Deposit '
'1196374984'
'5,000.00'
'5,000.00'       <- balance column, swallowed by "collect until next date or EOF" loop

'05/02'
'Venmo '
'Cashout '
'PPD ID: 5264681992'
'500.00'
'5,500.00'
```

The existing parser reads everything after the MM/DD line up to the first
amount; that gives us `"Venmo Cashout PPD ID: 5264681992"` which is what
we want to land in `src_chase.description`.

## CSV shapes

### Credit-card CSV header
```
Transaction Date,Post Date,Description,Category,Type,Amount,Memo
```

### Banking CSV header
```
Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #
```

Column names differ. Parser picks `Transaction Date` OR `Posting Date`,
whichever is present, and reads `Description` + `Amount` directly.

## source_tab rule — worked examples

9878 statements close on the 7th of each month. For a transaction dated
`2023-04-29`, the filenames available are `230407-...`, `230507-...`,
`230607-...`. The smallest statement-date `>= 2023-04-29` is `230507`, so
`source_tab = 230507-chase-9878.pdf`. The buggy importer left it on
`230407`, which is the statement that CLOSED 3 weeks *before* the
transaction — hence the wrong PDF opening.

Close dates drift day-by-day across months (e.g. Apr 7, May 7, Jun 7
vs. checking closings on the 8th–11th). The rule is per-last4, not
global, so the importer groups processed filenames by last4 before
picking.

## "Looks machine-generated" recognizer

The backfill overwrites `transactions.item` only when the current value
looks like it came from the importer. The recognizer accepts any of:

1. Exact match with `normalize_item(description)`.
2. Exact match with the raw description (whitespace-collapsed).
3. Exact match with the output of the old buggy greedy regex
   `\s+[A-Z][A-Za-z. ]+\s+[A-Z]{2}$` against the raw description (this is
   how rows like `WESTLAKE AQUATIC &` got truncated — we recognize and
   repair them).
4. Exact match with the output of the earlier single-word-city stripper
   `\s+[A-Za-z][A-Za-z'-]*\s+[A-Z]{2}$`.
5. Case-and-punctuation-insensitive prefix of the raw description (for
   legacy rows like `GITHUB INC.` from `GITHUB, INC. HTTPSGITHUB.C CA`).

Anything else is presumed user-edited and left alone.
