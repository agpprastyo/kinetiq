// SSO/SAML/OIDC/LDAP/SCIM plugin stubs for Fase 29 (self-hosted)
export interface SSOConfig {
  enabled: boolean;
  provider: "saml" | "oidc" | "ldap" | null;
}

export function getSSOConfig(): SSOConfig {
  return {
    enabled: process.env.SSO_ENABLED === "true",
    provider: (process.env.SSO_PROVIDER as any) || null,
  };
}

export function enforceSSOOnly() {
  return process.env.SSO_ONLY === "true";
}
