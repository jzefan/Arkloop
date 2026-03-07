import { zhCN } from './zh-CN'
import { en } from './en'

export type Locale = 'zh' | 'en'

export interface LocaleStrings {
  nav: {
    dashboard: string
    agents: string
    models: string
    tools: string
    runs: string
    settings: string
  }
  dashboard: {
    title: string
    runsTotal: string
    runsToday: string
    inputTokens: string
    outputTokens: string
    tokenUsage30d: string
    refresh: string
  }
  agents: {
    title: string
    newAgent: string
    editAgent: string
    name: string
    model: string
    systemPrompt: string
    tools: string
    setDefault: string
    active: string
    advanced: string
    temperature: string
    maxOutputTokens: string
    reasoningMode: string
    reasoningDisabled: string
    reasoningEnabled: string
    deleteConfirm: string
    noAgents: string
    overview: string
    persona: string
    builtIn: string
    platformDefault: string
    hybrid: string
    toolsSelected: (selected: number, total: number) => string
    enableAllTools: string
    clearAllTools: string
    groupEnableAll: string
    groupClearAll: string
  }
  models: {
    title: string
    searchProvider: string
    addProvider: string
    addProviderTitle: string
    editModel: string
    name: string
    apiKey: string
    baseUrl: string
    clientType: string
    clientTypeOpenaiResponse: string
    clientTypeOpenaiChat: string
    clientTypeAnthropic: string
    saveChanges: string
    deleteProvider: string
    deleteProviderConfirm: (name: string) => string
    modelsSection: string
    addModel: string
    importModels: string
    importModelsTitle: string
    importModelsLoading: string
    importModelsEmpty: string
    importModelsError: string
    importSelected: string
    importSearchPlaceholder: string
    modelName: string
    tags: string
    addTag: string
    setDefault: string
    deleteModel: string
    deleteModelConfirm: (model: string) => string
    modelSettings: string
    currentKeyPrefix: string
    errNameRequired: string
    errApiKeyRequired: string
    errModelRequired: string
    errProviderRequired: string
    noProviders: string
    toastCreated: string
    toastUpdated: string
    toastDeleted: string
    toastFailed: string
    toastLoadFailed: string
    toastImported: string
  }
  tools: {
    title: string
    sectionProvider: string
    sectionConfig: string
    sectionPool: string
    sectionTimeout: string
    sectionToolDescriptions: string
    statusActive: string
    statusInactive: string
    statusUnconfigured: string
    activate: string
    deactivate: string
    configure: string
    clearCredential: string
    clearTitle: string
    clearMessage: (providerName: string) => string
    clearConfirm: string
    modalTitle: string
    fieldApiKey: string
    fieldBaseUrl: string
    fieldBaseUrlOptional: string
    currentKeyPrefix: string
    errApiKeyRequired: string
    errBaseUrlRequired: string
    fieldAllowEgress: string
    fieldDockerImage: string
    fieldMaxSessions: string
    fieldBootTimeout: string
    fieldRefillInterval: string
    fieldRefillConcurrency: string
    fieldMaxLifetime: string
    fieldCostPerCommit: string
    fieldCostPerCommitHint: string
    editDescription: string
    resetDescription: string
    descriptionPlaceholder: string
    save: string
    cancel: string
    toastUpdated: string
    toastUpdateFailed: string
    toastSaved: string
    toastSaveFailed: string
    toastLoadFailed: string
  }
  common: {
    save: string
    cancel: string
    edit: string
    delete: string
    confirm: string
    loading: string
    default: string
    signOut: string
  }
  // auth
  loginMode: string
  enterYourPasswordTitle: string
  fieldIdentity: string
  fieldPassword: string
  identityPlaceholder: string
  enterPassword: string
  continueBtn: string
  backBtn: string
  editIdentity: string
  useEmailOtpHint: string
  otpLoginTab: string
  otpEmailPlaceholder: string
  otpCodePlaceholder: string
  otpSendBtn: string
  otpSendingCountdown: (s: number) => string
  otpVerifyBtn: string
  requestFailed: string
  loading: string
  // access denied
  accessDenied: string
  noAdminAccess: string
  signOut: string
  // settings modal
  account: string
  settings: string
  language: string
  appearance: string
  themeSystem: string
  themeLight: string
  themeDark: string
}

export const locales: Record<Locale, LocaleStrings> = { zh: zhCN, en }
