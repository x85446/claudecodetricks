# Concurrent React in React Native

## Available Features

React 18+ concurrent features available in React Native with New Architecture:

### useTransition
Mark non-urgent state updates as transitions to keep UI responsive.

```typescript
import { useTransition } from 'react';

const SearchScreen = () => {
  const [isPending, startTransition] = useTransition();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);

  const handleSearch = (text: string) => {
    setQuery(text); // Urgent: update input immediately
    startTransition(() => {
      setResults(filterResults(text)); // Non-urgent: can be interrupted
    });
  };

  return (
    <>
      <TextInput value={query} onChangeText={handleSearch} />
      {isPending ? <ActivityIndicator /> : <ResultsList data={results} />}
    </>
  );
};
```

### useDeferredValue
Defer expensive re-renders.

```typescript
import { useDeferredValue } from 'react';

const ProjectList = ({ filter }: { filter: string }) => {
  const deferredFilter = useDeferredValue(filter);
  const isStale = filter !== deferredFilter;

  const filtered = useMemo(
    () => projects.filter(p => p.name.includes(deferredFilter)),
    [deferredFilter]
  );

  return (
    <View style={[styles.list, isStale && styles.stale]}>
      <FlatList data={filtered} renderItem={renderItem} />
    </View>
  );
};
```

### Suspense (Limited in RN)
Works with React.lazy for code splitting:

```typescript
const HeavyChart = React.lazy(() => import('./HeavyChart'));

const AnalyticsScreen = () => (
  <Suspense fallback={<ChartSkeleton />}>
    <HeavyChart data={chartData} />
  </Suspense>
);
```

## When to Use

| Pattern | Use Case | RN Support |
|---------|----------|------------|
| `useTransition` | Search, filters, tabs | Full (New Arch) |
| `useDeferredValue` | Expensive list filtering | Full (New Arch) |
| `Suspense` | Code splitting, lazy screens | Partial |
| `React.lazy` | Screen-level code splitting | Full |

## Caveats in React Native
- Concurrent features require New Architecture (Fabric + TurboModules)
- `Suspense` for data fetching works best with React Query/TanStack Query
- Don't over-use transitions - measure first, apply where needed
