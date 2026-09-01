# FlatList & FlashList Optimization

## Core Rules

1. **NEVER use ScrollView + .map() for dynamic lists** - Always FlatList or FlashList
2. **Always provide keyExtractor** - Unique, stable keys
3. **Use getItemLayout for fixed-height items** - Eliminates measurement passes
4. **Wrap renderItem with React.memo or useCallback** - Prevents re-renders
5. **Consider FlashList for 100+ item lists** - Recycler-based, significantly faster

## Required FlatList Props (Large Lists)

```typescript
<FlatList
  data={items}
  renderItem={renderItem}
  keyExtractor={item => item.id}
  getItemLayout={(_, index) => ({
    length: ITEM_HEIGHT,
    offset: ITEM_HEIGHT * index,
    index,
  })}
  windowSize={11}                    // Reduce from default 21
  maxToRenderPerBatch={10}           // Batch rendering
  updateCellsBatchingPeriod={50}     // ms between batch renders
  removeClippedSubviews={Platform.OS === 'android'}
  initialNumToRender={10}            // First render count
/>
```

## FlashList Migration

```typescript
import { FlashList } from '@shopify/flash-list';

<FlashList
  data={items}
  renderItem={renderItem}
  keyExtractor={item => item.id}
  estimatedItemSize={ITEM_HEIGHT}    // Required for FlashList
  drawDistance={250}                   // Pixels ahead to render
/>
```

## Render Item Optimization

```typescript
// WRONG - New function every render
<FlatList renderItem={({ item }) => <Card item={item} />} />

// CORRECT - Stable reference
const renderItem = useCallback(
  ({ item }: { item: ItemType }) => <MemoizedCard item={item} />,
  []
);

// MemoizedCard.tsx
const Card = React.memo(({ item }: { item: ItemType }) => (
  <View style={styles.card}>
    <Text>{item.title}</Text>
  </View>
));
```

## Common Pitfalls

- **Inline styles in renderItem** - Create StyleSheet outside component
- **Anonymous functions as props** - Use useCallback or extract
- **Missing key changes** - keyExtractor must return strings
- **Nested FlatLists** - Use SectionList instead, or set `nestedScrollEnabled`
- **Heavy item components** - Extract sub-components, lazy-load images

## Measurement

```bash
# Enable FPS monitor
# In dev menu: "Show Perf Monitor"
# Target: 60fps during fast scroll
# Warning: <45fps during scroll
# Critical: <30fps during scroll
```
