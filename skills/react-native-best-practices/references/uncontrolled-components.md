# Uncontrolled Components & Ref Patterns

## When to Use Refs Instead of State

Use refs when the value doesn't need to trigger re-renders:

| Use Case | State | Ref |
|----------|-------|-----|
| Display in UI | ✅ | ❌ |
| Animation values | ❌ | ✅ (useSharedValue) |
| Timer IDs | ❌ | ✅ |
| Previous value tracking | ❌ | ✅ |
| TextInput intermediate value | ❌ | ✅ (with onEndEditing) |
| Scroll position | ❌ | ✅ |

## TextInput Pattern

```typescript
// PROBLEM: Re-renders on every keystroke
const [text, setText] = useState('');
<TextInput value={text} onChangeText={setText} />

// BETTER: Ref for intermediate, state for final
const textRef = useRef('');
const [submittedText, setSubmittedText] = useState('');

<TextInput
  defaultValue=""
  onChangeText={(t) => { textRef.current = t; }}
  onEndEditing={() => setSubmittedText(textRef.current)}
/>
```

## Scroll Position Tracking

```typescript
const scrollY = useRef(0);

const handleScroll = useCallback((event: NativeSyntheticEvent<NativeScrollEvent>) => {
  scrollY.current = event.nativeEvent.contentOffset.y;
  // No re-render! Use scrollY.current when needed
}, []);

<FlatList onScroll={handleScroll} scrollEventThrottle={16} />
```

## Debounced Search with Ref

```typescript
const SearchInput = ({ onSearch }: { onSearch: (q: string) => void }) => {
  const timerRef = useRef<NodeJS.Timeout>();

  const handleChange = useCallback((text: string) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => onSearch(text), 300);
  }, [onSearch]);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  return <TextInput onChangeText={handleChange} />;
};
```

## Previous Value Hook

```typescript
function usePrevious<T>(value: T): T | undefined {
  const ref = useRef<T>();
  useEffect(() => {
    ref.current = value;
  });
  return ref.current;
}

// Usage
const prevCount = usePrevious(count);
if (prevCount !== undefined && count > prevCount) {
  // Count increased
}
```
