# Frontend Authentication Architecture

## Component Architecture

```mermaid
graph TB
    subgraph "🌐 Frontend Application"
        subgraph "📱 React/Preact Components"
            LoginPage[🔑 LoginPageMinimal]
            ProfilePage[👤 ProfilePage] 
            HomePage[🏠 HomePage]
            AppDetails[📋 AppDetailsMinimal<br/>🐛 Fixed response parsing]
            PublicToggle[🔓 Public App Toggle<br/>✅ Correct status display]
            PrivateRoute[🛡️ PrivateRoute]
            GitHubOAuth[🔗 GitHubOAuth Component]
        end

        subgraph "🔄 Context & State Management"
            AuthContext[🎯 AuthContext Provider<br/>• Pure SSO session based<br/>• No JWT token handling]
            AuthState[Authentication State<br/>isAuthenticated<br/>isLoading<br/>user<br/>ssoSession (cookie-only)]
        end

        subgraph "🔧 Hooks & Utilities (🐛 FIXED)"
            useAuth[🪝 useAuth Hook]
            useApi[🌐 useApi Hook<br/>🐛 FIXED: Response parsing<br/>✅ response.property<br/>❌ response.data.property]
            AxiosInterceptors[⚡ Axios Interceptors<br/>• withCredentials: true<br/>• No token headers]
            TypeSafeAPI[🎯 TypeScript Interfaces<br/>✅ Correct activity types<br/>✅ Public setting types]
        end

        subgraph "💾 Storage Management"
            CookieUtils[🍪 Cookie Utilities<br/>• HttpOnly support<br/>• No localStorage fallback]
            SessionHelpers[🔧 Session Helpers<br/>• getSession - cookie only<br/>• No URL parameter parsing]
        end

        subgraph "🛣️ Routing & Navigation"
            Router[🗺️ App Router]
            RouteGuards[🛡️ Route Guards]
        end

        subgraph "⚙️ App Management"
            PublicSettings[🔓 Public App Settings<br/>• Toggle public/private<br/>• Real-time updates<br/>• Default private handling]
            AppConfig[📊 App Configuration<br/>• Settings management<br/>• Status synchronization]
        end
    end

    subgraph "🌍 Browser Environment"
        BrowserCookies[🍪 Browser Cookies<br/>• sso_session (HttpOnly)<br/>• Secure flag<br/>• SameSite protection<br/>• NO JWT storage]
        ErrorHandling[🚨 Error Management<br/>• Proper fallbacks<br/>• User feedback<br/>• Debug logging]
    end

    subgraph "🖥️ Backend Integration"
        AuthAPI[🔌 Auth API Endpoints<br/>• /auth/login<br/>• /auth/logout<br/>• /auth/token-validate (SSO)]
        AppAPI[📱 App API Endpoints<br/>• /apps/{app}/public-setting<br/>• /apps/{app}/activities<br/>✅ Fixed response parsing]
        GitHubAPI[🔗 GitHub API Endpoints<br/>• /github/auth/init<br/>• /github/auth/callback]
    end

    %% Authentication Flow (SSO Only)
    LoginPage -->|Login Form Submit| useApi
    useApi -->|🐛 FIXED: Direct response access| useApi
    useApi -->|POST /auth/login| AuthAPI
    AuthAPI -->|Success + SSO Session| useApi
    useApi -->|✅ response.sso_session| LoginPage
    LoginPage -->|Call login method| AuthContext
    AuthContext -->|Update State| AuthState
    AuthContext -->|Store Session| SessionHelpers
    SessionHelpers -->|HttpOnly Cookie Storage| BrowserCookies

    %% Public App Management Flow
    AppDetails -->|Load Public Setting| useApi
    useApi -->|GET /apps/{app}/public-setting| AppAPI
    AppAPI -->|✅ Fixed: Direct response| useApi
    useApi -->|✅ response.is_public| PublicSettings
    PublicSettings -->|Display Status| PublicToggle
    PublicToggle -->|Toggle Status| useApi
    useApi -->|POST /apps/{app}/public-setting| AppAPI
    AppAPI -->|✅ Setting updated| PublicSettings

    %% Activity Management (Fixed)
    AppDetails -->|Load Activities| useApi
    useApi -->|GET /apps/{app}/activities| AppAPI
    AppAPI -->|✅ Fixed: {activities: [...], total: N}| useApi
    useApi -->|✅ response.activities| AppDetails
    AppDetails -->|✅ Display correctly| AppDetails

    %% Session Initialization (Cookie-Only)
    AuthContext -->|Initialize| BrowserCookies
    BrowserCookies -->|Read HttpOnly Cookie| AuthContext
    AuthContext -->|Check Storage| SessionHelpers
    SessionHelpers -->|✅ Cookie-only validation| AuthContext
    
    %% Session Validation (No JWT)
    AuthContext -->|checkAuth method| useApi
    useApi -->|GET /auth/token-validate| AuthAPI
    AuthAPI -->|SSO Session Validation| useApi
    useApi -->|✅ response.user_id/username| AuthContext

    %% Error Handling & Type Safety
    useApi -->|Parse Error| ErrorHandling
    ErrorHandling -->|Type-Safe Errors| TypeSafeAPI
    TypeSafeAPI -->|Proper Types| AppDetails
    AppDetails -->|Safe Property Access| PublicSettings

    %% Route Protection
    Router -->|Route Change| PrivateRoute
    PrivateRoute -->|Check Auth| useAuth
    useAuth -->|Get State| AuthContext
    AuthContext -->|Redirect if needed| LoginPage

    %% Components Auth Access
    HomePage -->|Get Auth Data| useAuth
    ProfilePage -->|Get Auth Data| useAuth
    GitHubOAuth -->|API Calls| useApi
    AppDetails -->|✅ Fixed API calls| useApi

    %% API Communication (Fixed)
    useApi -->|✅ Correct parsing| AxiosInterceptors
    AxiosInterceptors -->|withCredentials true| AuthAPI
    AxiosInterceptors -->|✅ No token headers| AppAPI
    AxiosInterceptors -->|Auto-retry on 401| AuthContext

    %% Session Management (No JWT)
    AuthContext -->|✅ SSO-only logout| useApi
    useApi -->|POST /auth/logout| AuthAPI
    AuthContext -->|Clear State| SessionHelpers
    SessionHelpers -->|Clear HttpOnly Storage| BrowserCookies

    %% GitHub OAuth Flow
    GitHubOAuth -->|Init OAuth| useApi
    useApi -->|GET /github/auth/init| GitHubAPI
    GitHubAPI -->|OAuth URL| useApi
    useApi -->|✅ response.auth_url| GitHubOAuth
    GitHubOAuth -->|Handle Callback| GitHubAPI
    
    classDef component fill:#e3f2fd
    classDef context fill:#f3e5f5
    classDef hook fill:#e8f5e8
    classDef storage fill:#fff3e0
    classDef routing fill:#fce4ec
    classDef browser fill:#f1f8e9
    classDef backend fill:#ffecb3
    classDef management fill:#f0f4c3
    classDef fixed fill:#c8e6c9

    class LoginPage,ProfilePage,HomePage,PrivateRoute,GitHubOAuth component
    class AppDetails,PublicToggle fixed
    class AuthContext,AuthState context
    class useAuth,useApi,AxiosInterceptors,TypeSafeAPI hook
    class CookieUtils,SessionHelpers storage
    class Router,RouteGuards routing
    class BrowserCookies,ErrorHandling browser
    class AuthAPI,GitHubAPI,AppAPI backend
    class PublicSettings,AppConfig management
```

## Major Frontend Improvements

### 🐛 useApi Hook Bug Fixes
**Critical Response Parsing Fix:**
```javascript
// ❌ WRONG (Previous):
if (response && response.data) {
  setIsPublic(response.data.is_public);  // ✗ Undefined!
}

// ✅ CORRECT (Fixed):
if (response) {
  setIsPublic(response.is_public);  // ✓ Works!
}
```

**Root Cause**: useApi hook already extracts `data` from backend response `{success, message, data}`, so components should access properties directly from response, not `response.data.property`.

### 🔓 Public App Management
- **Correct Status Display**: Fixed default private status handling
- **Real-time Updates**: Immediate UI updates on status change
- **Type Safety**: Proper TypeScript interfaces for public settings
- **Error Handling**: Graceful fallback for missing settings

### 🎯 TypeScript Improvements
- **Activity Types**: Fixed `useApi<{activities: any[], total: number}>()`
- **Response Interfaces**: Correct typing for all API responses
- **Error Prevention**: Type-safe property access throughout
- **Build Fixes**: Eliminated TypeScript compilation errors

### 🚫 Complete JWT Removal
- **Cookie-Only Storage**: No localStorage, sessionStorage, or URL parameters
- **HttpOnly Cookies**: Enhanced security with browser-only access
- **Simplified Auth**: Pure SSO session-based authentication
- **No Token Headers**: Removed all JWT/Bearer token handling

### 🔐 Enhanced Security
- **SameSite Protection**: CSRF protection for cross-site requests  
- **Secure Flag**: HTTPS-only cookie transmission
- **HttpOnly Flag**: JavaScript cannot access session data
- **Domain Isolation**: Secure cookie handling for custom domains

### ⚡ Performance Optimizations
- **Direct Property Access**: Faster response parsing with fixed useApi
- **Reduced Error Handling**: Fewer try-catch blocks needed
- **Type Safety**: Compile-time error prevention
- **Memory Efficiency**: No redundant data structures

## Critical Bug Patterns Fixed

| Component | Bug Pattern | Fix Applied |
|-----------|-------------|-------------|
| `AppDetailsMinimal` | `response.data.is_public` | `response.is_public` |
| `AppDetailsMinimal` | `(result as any).activities` | `result.activities` |
| `GitHubOAuth` | `response.data.auth_url` | `response.auth_url` |
| `DockerConnection` | `response.data.connected` | `response.connected` |
| All Components | Mixed response parsing | Consistent direct access |

## Description
Enhanced frontend authentication architecture with critical bug fixes, public app management, and complete JWT removal. Features improved TypeScript safety, proper error handling, and secure cookie-only session management. 