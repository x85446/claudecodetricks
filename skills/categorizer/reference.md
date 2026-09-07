# Categorizer reference — categories, cost centers, lookup SQL

The DB is the source of truth; these tables are a convenience snapshot. If they
disagree with the DB, regenerate from the DB. Full hierarchy doc: `docs/tier-system.md`.

## DB access

```python
import sqlite3
conn = sqlite3.connect('db/personaldb.sqlite')
conn.execute("PRAGMA foreign_keys=ON")
conn.row_factory = sqlite3.Row
```

Writes touch FK columns only (text mirrors were dropped 2026-04-10):

```sql
UPDATE transactions SET
  tier1_id = ?, tier2_id = ?, tier3_id = ?,
  updated_at = datetime('now')
WHERE id = ?;
```

## Category lookup

```sql
SELECT t1.id t1_id, t1.name tier1, t2.id t2_id, t2.name tier2, t3.id t3_id, t3.name tier3
FROM categories_tier1 t1
LEFT JOIN categories_tier2 t2 ON t2.tier1_id = t1.id
LEFT JOIN categories_tier3 t3 ON t3.tier2_id = t2.id
WHERE t1.name LIKE '%<kw>%' OR t2.name LIKE '%<kw>%' OR t3.name LIKE '%<kw>%';
```

Tree ownership: every tier1 belongs to one cost center (`categories_tier1.company_id`);
properties share MAPT's tree via `companies.tree_owner_id`. Validate a category is
in the row's tree before writing.

## Cost centers

| Type | Cost centers |
|---|---|
| Personal | `-` (company_id = 1) |
| LLCs | MAPT (2021-03-09), TMCTECH (2021-12-28), T&ETECH (2022), IZUMA (2022), GRAVHL (2022-03-31), MAPTTW (2024-06-10), REDRIVER |
| Properties | 1913, 1028, 305, 502, 7207, 6505 (MAPT's tree) |

Only IZUMA and GRAVHL use expense reports. Business tier1 names carry a `- ` prefix.
Capital contributions into a business: Personal, `Finance / Investment / <cost center>`.

## Personal tier1 categories (17) and their tier2 sets

| Tier1 | Tier2 (complete, case-exact) |
|---|---|
| Bills & Utilities | Cleaning, Internet, Mobile Phone, Utilities, pest |
| Clothing | Accessories, Adult clothing, Children's Clothing, Cleaners, Men's Clothing, Other, Shoes, Swimwear, Workout clothing |
| Debt & Insurance | CC benefits, Credit card payments, Insurance, Monitoring SW, Mortgage, Personal loans, Student loans |
| Education | Extracurricular activities, Other, School supplies, Tuition |
| Electronics | Audio and home entertainment, Batteries, Cameras and photography, Computers, Home appliances, Home automation, Microelectronics and soldering, Networking, Phones, Racking Cases Storage Organization, Software, TVs and home theater, Video games and consoles, cables, other |
| Entertainment | Books, Camps, Hobbies and activities, Mobile apps, Movie nights, Music, Other, Streaming services, Workout clothing, sporting-event |
| Finance | Business Payment, HOA, Interest, Investment, Mortgage, Other, Other Income, Paycheck, Realtor fees, fee, reimbursement, rental income, tax, transfer-in, transfer-out |
| Food | Coffee, Dining out, Groceries, Other, School lunches, Snacks, Takeout or delivery, drinks, vitamins |
| Health | Dental care, Eye care, Fitness, Medical devices, Other, Out-of-pocket medical expenses, Personal care items, Prescription medications, Rx, Sports, copays, online provider, services, vitamins |
| Home | Building, Improvement, Kitchenware, Maintenance, Repairs, Small appliances, Storage & Organization, Yard, bedding, cleaning supplies, cleaning tools, electrical, indoor furniture, other, outdoor furniture |
| Home Improvement | Building, Built in furniture & structure, Electrical, Framing, Kitchen structure, Lighting, Plumbing, Storage, Supplies, Wiring, Yard |
| Kids | Activites, EEF, Education, Other, babysitter, gifts, grades, teachers, toys |
| Miscellaneous | Donations, Gifts, Pet, Shipping, Subscriptions, other |
| Office & Software | Domain Names, Other, SaaS COGS, SaaS OPS, Software, Storage, furniture, supplies |
| Tools & Hardware | Consumables, Fasteners, Hand tools, Other, Power tools, Tool storage |
| Transportation | Accessories, Car Insurance, Car maintenance and repairs, Car payment, Fuel, Other, Parking, Public transportation, car services, registrations, tolls |
| Travel | Accommodations, Activities and entertainment, Food and dining, Other, Transportation, Vacations, luggage, tolls |

Home vs Home Improvement: Home = household goods/upkeep; Home Improvement =
construction/renovation.

## Retired categories — never use

FIXME, Payment (→ Debt & Insurance), Household (→ Home), Debt (→ Debt & Insurance),
Fitness & Sports (→ Health, tier2 Fitness|Sports), Pet as tier1 (→ Miscellaneous/Pet),
transfers (→ transfer-in | transfer-out).

## Useful integrity queries

```sql
-- tier2 under wrong tier1 / tier3 under wrong tier2
SELECT t.id FROM transactions t JOIN categories_tier2 c2 ON c2.id = t.tier2_id
WHERE c2.tier1_id != t.tier1_id;
SELECT t.id FROM transactions t JOIN categories_tier3 c3 ON c3.id = t.tier3_id
WHERE c3.tier2_id != t.tier2_id;

-- formation-date violations (example: MAPTTW)
SELECT t.id FROM transactions t JOIN companies c ON c.id = t.company_id
WHERE c.code = 'MAPTTW' AND t.date < '2024-06-10';

-- category outside the row's cost-center tree
SELECT t.id FROM transactions t
JOIN categories_tier1 c1 ON c1.id = t.tier1_id
JOIN companies co ON co.id = t.company_id
WHERE c1.company_id != COALESCE(co.tree_owner_id, co.id);
```
