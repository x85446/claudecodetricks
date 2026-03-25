
// this file is generated — do not edit it


/// <reference types="@sveltejs/kit" />

/**
 * Environment variables [loaded by Vite](https://vitejs.dev/guide/env-and-mode.html#env-files) from `.env` files and `process.env`. Like [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), this module cannot be imported into client-side code. This module only includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured).
 * 
 * _Unlike_ [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), the values exported from this module are statically injected into your bundle at build time, enabling optimisations like dead code elimination.
 * 
 * ```ts
 * import { API_KEY } from '$env/static/private';
 * ```
 * 
 * Note that all environment variables referenced in your code should be declared (for example in an `.env` file), even if they don't have a value until the app is deployed:
 * 
 * ```
 * MY_FEATURE_FLAG=""
 * ```
 * 
 * You can override `.env` values from the command line like so:
 * 
 * ```bash
 * MY_FEATURE_FLAG="enabled" npm run dev
 * ```
 */
declare module '$env/static/private' {
	export const npm_package_engines_npm: string;
	export const NVM_INC: string;
	export const TERM_PROGRAM: string;
	export const rvm_bin_path: string;
	export const NODE: string;
	export const npm_config_audit: string;
	export const INIT_CWD: string;
	export const NVM_CD_FLAGS: string;
	export const GEM_HOME: string;
	export const TERM: string;
	export const SHELL: string;
	export const EVIDENCE_DATA_URL_PREFIX: string;
	export const IRBRC: string;
	export const HOMEBREW_REPOSITORY: string;
	export const TMPDIR: string;
	export const GITHUB_DOCKER_HUB: string;
	export const npm_config_global_prefix: string;
	export const DIRENV_DIR: string;
	export const TERM_PROGRAM_VERSION: string;
	export const NODE_OPTIONS: string;
	export const GITHUB_GHCR_TOKEN: string;
	export const COLOR: string;
	export const MY_RUBY_HOME: string;
	export const TERM_SESSION_ID: string;
	export const npm_config_noproxy: string;
	export const npm_config_local_prefix: string;
	export const LC_ALL: string;
	export const HISTFILESIZE: string;
	export const USER: string;
	export const NVM_DIR: string;
	export const COMMAND_MODE: string;
	export const OPENAI_API_KEY: string;
	export const npm_config_globalconfig: string;
	export const rvm_path: string;
	export const OPENAI_KEY: string;
	export const SSH_AUTH_SOCK: string;
	export const EVIDENCE_DATA_DIR: string;
	export const __CF_USER_TEXT_ENCODING: string;
	export const npm_execpath: string;
	export const PYENV_VIRTUALENV_INIT: string;
	export const TERM_FEATURES: string;
	export const VIRTUAL_ENV: string;
	export const DIRENV_WATCHES: string;
	export const PYENV_VIRTUAL_ENV: string;
	export const SUBLWORKDIR: string;
	export const rvm_prefix: string;
	export const PATH: string;
	export const TERMINFO_DIRS: string;
	export const npm_package_json: string;
	export const _: string;
	export const LaunchInstanceID: string;
	export const npm_config_userconfig: string;
	export const npm_config_init_module: string;
	export const __CFBundleIdentifier: string;
	export const npm_command: string;
	export const thishome: string;
	export const PWD: string;
	export const OPENROUTER_API_KEY: string;
	export const npm_lifecycle_event: string;
	export const EDITOR: string;
	export const npm_package_name: string;
	export const LANG: string;
	export const GITHUB_ORGANIZATION: string;
	export const ITERM_PROFILE: string;
	export const npm_config_npm_version: string;
	export const _OLD_VIRTUAL_PS1: string;
	export const XPC_FLAGS: string;
	export const npm_package_engines_node: string;
	export const ANTHROPIC_API_KEY: string;
	export const npm_config_node_gyp: string;
	export const MANU_LIBS_PATH: string;
	export const npm_package_version: string;
	export const XPC_SERVICE_NAME: string;
	export const DIRENV_FILE: string;
	export const KEYS: string;
	export const rvm_version: string;
	export const SHLVL: string;
	export const COLORFGBG: string;
	export const HOME: string;
	export const PYENV_SHELL: string;
	export const SHELL_SESSION_HISTORY: string;
	export const LANGUAGE: string;
	export const npm_config_loglevel: string;
	export const LC_TERMINAL_VERSION: string;
	export const HOMEBREW_PREFIX: string;
	export const GITHUB_USER: string;
	export const ITERM_SESSION_ID: string;
	export const npm_config_cache: string;
	export const GITHUB_SUPER_TOKEN: string;
	export const LOGNAME: string;
	export const npm_lifecycle_script: string;
	export const GEM_PATH: string;
	export const npm_config_fund: string;
	export const NVM_BIN: string;
	export const npm_config_user_agent: string;
	export const PROMPT_COMMAND: string;
	export const HOMEBREW_CELLAR: string;
	export const INFOPATH: string;
	export const SERVICE_ACCOUNT_PATH: string;
	export const DRIVE_FOLDER_ID: string;
	export const DISPLAY: string;
	export const LC_TERMINAL: string;
	export const OSLogRateLimit: string;
	export const DIRENV_DIFF: string;
	export const SECURITYSESSIONID: string;
	export const RUBY_VERSION: string;
	export const HISTFILE: string;
	export const OCTA_API_KEY: string;
	export const npm_node_execpath: string;
	export const npm_config_prefix: string;
	export const COLORTERM: string;
	export const NODE_ENV: string;
}

/**
 * Similar to [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private), except that it only includes environment variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`), and can therefore safely be exposed to client-side code.
 * 
 * Values are replaced statically at build time.
 * 
 * ```ts
 * import { PUBLIC_BASE_URL } from '$env/static/public';
 * ```
 */
declare module '$env/static/public' {
	
}

/**
 * This module provides access to runtime environment variables, as defined by the platform you're running on. For example if you're using [`adapter-node`](https://github.com/sveltejs/kit/tree/main/packages/adapter-node) (or running [`vite preview`](https://svelte.dev/docs/kit/cli)), this is equivalent to `process.env`. This module only includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured).
 * 
 * This module cannot be imported into client-side code.
 * 
 * Dynamic environment variables cannot be used during prerendering.
 * 
 * ```ts
 * import { env } from '$env/dynamic/private';
 * console.log(env.DEPLOYMENT_SPECIFIC_VARIABLE);
 * ```
 * 
 * > In `dev`, `$env/dynamic` always includes environment variables from `.env`. In `prod`, this behavior will depend on your adapter.
 */
declare module '$env/dynamic/private' {
	export const env: {
		npm_package_engines_npm: string;
		NVM_INC: string;
		TERM_PROGRAM: string;
		rvm_bin_path: string;
		NODE: string;
		npm_config_audit: string;
		INIT_CWD: string;
		NVM_CD_FLAGS: string;
		GEM_HOME: string;
		TERM: string;
		SHELL: string;
		EVIDENCE_DATA_URL_PREFIX: string;
		IRBRC: string;
		HOMEBREW_REPOSITORY: string;
		TMPDIR: string;
		GITHUB_DOCKER_HUB: string;
		npm_config_global_prefix: string;
		DIRENV_DIR: string;
		TERM_PROGRAM_VERSION: string;
		NODE_OPTIONS: string;
		GITHUB_GHCR_TOKEN: string;
		COLOR: string;
		MY_RUBY_HOME: string;
		TERM_SESSION_ID: string;
		npm_config_noproxy: string;
		npm_config_local_prefix: string;
		LC_ALL: string;
		HISTFILESIZE: string;
		USER: string;
		NVM_DIR: string;
		COMMAND_MODE: string;
		OPENAI_API_KEY: string;
		npm_config_globalconfig: string;
		rvm_path: string;
		OPENAI_KEY: string;
		SSH_AUTH_SOCK: string;
		EVIDENCE_DATA_DIR: string;
		__CF_USER_TEXT_ENCODING: string;
		npm_execpath: string;
		PYENV_VIRTUALENV_INIT: string;
		TERM_FEATURES: string;
		VIRTUAL_ENV: string;
		DIRENV_WATCHES: string;
		PYENV_VIRTUAL_ENV: string;
		SUBLWORKDIR: string;
		rvm_prefix: string;
		PATH: string;
		TERMINFO_DIRS: string;
		npm_package_json: string;
		_: string;
		LaunchInstanceID: string;
		npm_config_userconfig: string;
		npm_config_init_module: string;
		__CFBundleIdentifier: string;
		npm_command: string;
		thishome: string;
		PWD: string;
		OPENROUTER_API_KEY: string;
		npm_lifecycle_event: string;
		EDITOR: string;
		npm_package_name: string;
		LANG: string;
		GITHUB_ORGANIZATION: string;
		ITERM_PROFILE: string;
		npm_config_npm_version: string;
		_OLD_VIRTUAL_PS1: string;
		XPC_FLAGS: string;
		npm_package_engines_node: string;
		ANTHROPIC_API_KEY: string;
		npm_config_node_gyp: string;
		MANU_LIBS_PATH: string;
		npm_package_version: string;
		XPC_SERVICE_NAME: string;
		DIRENV_FILE: string;
		KEYS: string;
		rvm_version: string;
		SHLVL: string;
		COLORFGBG: string;
		HOME: string;
		PYENV_SHELL: string;
		SHELL_SESSION_HISTORY: string;
		LANGUAGE: string;
		npm_config_loglevel: string;
		LC_TERMINAL_VERSION: string;
		HOMEBREW_PREFIX: string;
		GITHUB_USER: string;
		ITERM_SESSION_ID: string;
		npm_config_cache: string;
		GITHUB_SUPER_TOKEN: string;
		LOGNAME: string;
		npm_lifecycle_script: string;
		GEM_PATH: string;
		npm_config_fund: string;
		NVM_BIN: string;
		npm_config_user_agent: string;
		PROMPT_COMMAND: string;
		HOMEBREW_CELLAR: string;
		INFOPATH: string;
		SERVICE_ACCOUNT_PATH: string;
		DRIVE_FOLDER_ID: string;
		DISPLAY: string;
		LC_TERMINAL: string;
		OSLogRateLimit: string;
		DIRENV_DIFF: string;
		SECURITYSESSIONID: string;
		RUBY_VERSION: string;
		HISTFILE: string;
		OCTA_API_KEY: string;
		npm_node_execpath: string;
		npm_config_prefix: string;
		COLORTERM: string;
		NODE_ENV: string;
		[key: `PUBLIC_${string}`]: undefined;
		[key: `${string}`]: string | undefined;
	}
}

/**
 * Similar to [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), but only includes variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`), and can therefore safely be exposed to client-side code.
 * 
 * Note that public dynamic environment variables must all be sent from the server to the client, causing larger network requests — when possible, use `$env/static/public` instead.
 * 
 * Dynamic environment variables cannot be used during prerendering.
 * 
 * ```ts
 * import { env } from '$env/dynamic/public';
 * console.log(env.PUBLIC_DEPLOYMENT_SPECIFIC_VARIABLE);
 * ```
 */
declare module '$env/dynamic/public' {
	export const env: {
		[key: `PUBLIC_${string}`]: string | undefined;
	}
}
