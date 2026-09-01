# Evaluating Dependency Size Impact

## Before Adding Any Dependency

### 1. Check Bundle Cost
```bash
# Check npm package size
npx package-phobia <package-name>

# Or use bundlephobia.com API
curl "https://bundlephobia.com/api/size?package=<package-name>"
```

### 2. Check for Lighter Alternatives
| Heavy Library | Size | Alternative | Size |
|--------------|------|-------------|------|
| moment | ~300KB | date-fns | ~10KB (tree-shakeable) |
| lodash | ~70KB | lodash-es (direct import) | ~2KB per function |
| axios | ~30KB | fetch (built-in) | 0KB |
| uuid | ~15KB | react-native-uuid | ~3KB |
| numeral | ~40KB | Intl.NumberFormat | 0KB |

### 3. Check Native Dependencies
Native dependencies add:
- Binary size (.so/.dylib)
- Build time
- Maintenance burden
- Potential compatibility issues

```bash
# Check if package has native code
ls node_modules/<package>/ios/ 2>/dev/null
ls node_modules/<package>/android/ 2>/dev/null
```

## Decision Framework

```
Need the functionality?
├── No → Don't add it
└── Yes → Can you write it in <50 lines?
    ├── Yes → Write it yourself
    └── No → Is there a built-in alternative?
        ├── Yes → Use built-in (fetch, Intl, etc.)
        └── No → Pick the smallest maintained option
            ├── <10KB → Add it
            ├── 10-50KB → Consider alternatives
            └── >50KB → Strong justification needed
```

## Monitoring Dependencies

```bash
# List all dependencies with sizes
npx depcheck  # Find unused dependencies

# Check for duplicate packages
npm ls --all | grep -E "deduped|overridden"

# Size of node_modules
du -sh node_modules/

# Top 20 largest packages
du -sh node_modules/* | sort -rh | head -20
```

## Dependency Hygiene

- **Audit quarterly**: Remove unused dependencies
- **Pin versions**: Avoid surprise size increases
- **Check changelogs**: Before updating (new deps may be added)
- **Prefer peer dependencies**: When building reusable code
- **One dependency per problem**: Avoid overlapping libraries
