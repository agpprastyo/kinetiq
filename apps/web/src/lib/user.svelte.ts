/**
 * Shared reactive user state & default Kinetiq avatar generator.
 * Ensures profile edits in /app/profile immediately reflect in the top bar without a page reload.
 */

export const DEFAULT_AVATAR_PRESETS = [
  {
    id: "emerald",
    label: "Emerald",
    url:
      "data:image/svg+xml;utf8," +
      encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120"><rect width="120" height="120" rx="60" fill="#127852"/><circle cx="60" cy="46" r="21" fill="#E3F2EC"/><path d="M25 108c6-21 21-33 35-33s29 12 35 33" fill="#E3F2EC"/></svg>`
      ),
  },
  {
    id: "mint",
    label: "Mint",
    url:
      "data:image/svg+xml;utf8," +
      encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120"><rect width="120" height="120" rx="60" fill="#1A9E6C"/><circle cx="60" cy="46" r="21" fill="#FFFFFF"/><path d="M25 108c6-21 21-33 35-33s29 12 35 33" fill="#FFFFFF"/></svg>`
      ),
  },
  {
    id: "forest",
    label: "Forest",
    url:
      "data:image/svg+xml;utf8," +
      encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120"><rect width="120" height="120" rx="60" fill="#111A15"/><circle cx="60" cy="46" r="21" fill="#22B57D"/><path d="M25 108c6-21 21-33 35-33s29 12 35 33" fill="#22B57D"/></svg>`
      ),
  },
];

export const DEFAULT_USER_AVATAR = DEFAULT_AVATAR_PRESETS[0].url;

export function getUserAvatar(user: { image?: string | null } | null | undefined): string {
  if (user?.image && user.image.trim().length > 0) {
    return user.image.trim();
  }
  return DEFAULT_USER_AVATAR;
}

class UserStore {
  current = $state<any>(null);

  set(u: any) {
    this.current = u;
  }

  patch(partial: Record<string, any>) {
    if (this.current) {
      this.current = { ...this.current, ...partial };
    }
  }
}

export const userStore = new UserStore();
