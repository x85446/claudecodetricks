# React Native Comprehensive Rules

Based on CodeCompass React Native rules - the most thorough framework-agnostic React Native rules document available. Covers every aspect of bare React Native development without Expo.

## When to Reference

- Starting any new React Native component, screen, or feature
- Making architecture decisions
- Code review checklist
- Onboarding new patterns

## Core Component Rules

### Functional Components Only
```typescript
// ALWAYS use functional components with TypeScript
interface Props {
  title: string;
  onPress: () => void;
  disabled?: boolean;
}

const MyButton: React.FC<Props> = ({ title, onPress, disabled = false }) => {
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [styles.button, pressed && styles.pressed]}
      accessibilityRole="button"
      accessibilityLabel={title}
      accessibilityState={{ disabled }}
    >
      <Text style={styles.text}>{title}</Text>
    </Pressable>
  );
};
```

### StyleSheet Rules
- ALWAYS use `StyleSheet.create()` - never inline objects
- Place StyleSheet at bottom of file, outside component
- Use `Platform.select()` for cross-platform styles
- Never use string values for numeric style props

```typescript
// WRONG
<View style={{ padding: 16, backgroundColor: '#fff' }} />

// CORRECT
const styles = StyleSheet.create({
  container: {
    padding: 16,
    backgroundColor: '#fff',
  },
});
```

## State Management Hierarchy

1. **Local state** (`useState`) - Component-specific, ephemeral
2. **React Query / TanStack Query** - Server state, caching, sync
3. **Zustand** - Global client state (auth, theme, preferences)
4. **Context** - Rarely, only for deeply-nested prop drilling

### Anti-Patterns
- Don't use Redux (Zustand is simpler for this project)
- Don't use Context for frequently-changing state
- Don't store server data in global state (use React Query)
- Don't use AsyncStorage for state (use MMKV for persistence)

## Navigation Patterns

### Type-Safe Navigation
```typescript
// src/navigation/types.ts
export type RootStackParamList = {
  Home: undefined;
  Profile: { userId: string };
  Project: { projectId: string; tab?: 'details' | 'bids' };
  Settings: undefined;
};

// Usage in screens
type Props = NativeStackScreenProps<RootStackParamList, 'Profile'>;

const ProfileScreen: React.FC<Props> = ({ route, navigation }) => {
  const { userId } = route.params;
  // ...
};
```

### Deep Linking
```typescript
const linking = {
  prefixes: ['gravhl://', 'https://gravhl.com'],
  config: {
    screens: {
      Home: '',
      Profile: 'profile/:userId',
      Project: 'project/:projectId',
    },
  },
};
```

## Platform-Specific Code

### Decision Tree
- **Minor style differences** → `Platform.select()`
- **Different component behavior** → `.ios.tsx` / `.android.tsx` files
- **Different native APIs** → Separate files with shared interface

### Platform.select() Pattern
```typescript
const styles = StyleSheet.create({
  shadow: Platform.select({
    ios: {
      shadowColor: '#000',
      shadowOffset: { width: 0, height: 2 },
      shadowOpacity: 0.1,
      shadowRadius: 4,
    },
    android: {
      elevation: 4,
    },
  }),
});
```

## Native Module Guidance

### Before Writing a Native Module
1. Check React Native community libraries first
2. Check if the API is available through React Native core
3. Consider if a JS solution works well enough
4. Only then write a native module

### New Architecture (Preferred)
- Use Turbo Modules for native functionality
- Use Fabric for custom native views
- Codegen from TypeScript specs

## Security Rules

### Storage
- **Secrets (tokens, keys)**: `react-native-keychain` (iOS Keychain / Android Keystore)
- **Preferences**: `react-native-mmkv` (encrypted option available)
- **NEVER**: AsyncStorage for sensitive data

### Network
- Enable SSL pinning for API calls
- Never log sensitive data (tokens, passwords)
- Validate all API response shapes

### Code
- Enable Hermes (bytecode, harder to reverse-engineer)
- Enable ProGuard/R8 for Android
- Never embed API keys in source (use .env with react-native-config)

## Testing Stack

| Type | Tool | Location |
|------|------|----------|
| Unit | Jest | Co-located `.test.ts` |
| Component | Jest + RNTL | Co-located `.test.tsx` |
| Hook | Jest + renderHook | Co-located `.test.ts` |
| E2E | Detox | `e2e/` directory |
| Snapshot | Jest | Sparingly, for atoms only |

## Project Structure Reference

```
src/
├── components/
│   ├── atoms/           # Button, Text, Icon, Badge
│   ├── molecules/       # FormField, SearchBar, Card
│   └── organisms/       # ProjectCard, ChatDrawer, BidModal
├── screens/             # HomeScreen, ProfileScreen
├── navigation/          # Navigators, route types
├── hooks/               # Custom hooks
├── stores/              # Zustand stores
├── lib/
│   ├── api/             # API clients
│   └── auth/            # SuperTokens helpers
├── types/               # Shared TypeScript types
├── constants/           # App constants
├── theme/               # Colors, typography, spacing
├── utils/               # Pure utility functions
└── assets/              # Images, fonts, SVGs
```
