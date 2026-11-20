<script lang="ts">
  import { onMount } from "svelte";
  import { authClient } from "$lib/auth-client";
  import { toSafeUserMessage } from "$lib/api";
  import {
    userStore,
    getUserAvatar,
    DEFAULT_USER_AVATAR,
  } from "$lib/user.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import {
    User,
    Camera,
    Lock,
    ShieldCheck,
    Monitor,
    Trash2,
    Check,
    AlertTriangle,
    Upload,
    Sparkles,
    Mail,
    KeyRound,
    Smartphone,
    ZoomIn,
    Move,
    X
  } from "lucide-svelte";

  let user = $state<any>(null);
  let currentSessionToken = $state<string>("");
  let sessions = $state<any[]>([]);
  let loading = $state(true);

  // Profile Identity states
  let displayName = $state("");
  let avatarUrl = $state("");
  let savingProfile = $state(false);
  let profileMsg = $state<{ type: "success" | "error"; text: string } | null>(null);
  let sendingVerifyEmail = $state(false);

  // Pre-Upload Image Crop states
  let cropImageEl = $state<HTMLImageElement | null>(null);
  let cropDataUrl = $state<string | null>(null);
  let cropZoom = $state(1.2);
  let cropOffsetX = $state(0);
  let cropOffsetY = $state(0);
  let isDraggingCrop = $state(false);
  let dragStartX = $state(0);
  let dragStartY = $state(0);
  let cropCanvasEl = $state<HTMLCanvasElement | null>(null);

  // Password Change states
  let currentPassword = $state("");
  let newPassword = $state("");
  let confirmPassword = $state("");
  let revokeOthersOnPwd = $state(true);
  let savingPassword = $state(false);
  let passwordMsg = $state<{ type: "success" | "error"; text: string } | null>(null);

  // Two-Factor Authentication (2FA) states
  let twoFactorPassword = $state("");
  let totpSetupUri = $state("");
  let totpBackupCodes = $state<string[]>([]);
  let totpVerifyCode = $state("");
  let twoFactorLoading = $state(false);
  let twoFactorMsg = $state<{ type: "success" | "error"; text: string } | null>(null);

  // Sessions states
  let revokingToken = $state<string | null>(null);

  const previewAvatarSrc = $derived(getUserAvatar({ image: avatarUrl }));

  async function loadUserAndSessions() {
    loading = true;
    try {
      const sessionRes = await authClient.getSession();
      if (sessionRes?.data?.user) {
        user = sessionRes.data.user;
        userStore.set(user);
        currentSessionToken = sessionRes.data.session?.token || "";
        displayName = user.name || "";
        avatarUrl = user.image || DEFAULT_USER_AVATAR;
      }

      const listRes = await authClient.listSessions();
      if (listRes?.data && Array.isArray(listRes.data)) {
        sessions = listRes.data;
      }
    } catch (e) {
      console.error("Failed to load profile", e);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadUserAndSessions();
  });

  function drawCropPreview() {
    if (!cropCanvasEl || !cropImageEl) return;
    const ctx = cropCanvasEl.getContext("2d");
    if (!ctx) return;

    const size = 256;
    ctx.clearRect(0, 0, size, size);

    const imgW = cropImageEl.naturalWidth || cropImageEl.width || 1;
    const imgH = cropImageEl.naturalHeight || cropImageEl.height || 1;

    // Base scale so shortest side fills the 256x256 square
    const baseScale = Math.max(size / imgW, size / imgH);
    const finalScale = baseScale * cropZoom;

    const drawW = imgW * finalScale;
    const drawH = imgH * finalScale;

    // Clamp offsets so the image always covers the square
    const maxOffsetX = Math.max(0, (drawW - size) / 2);
    const maxOffsetY = Math.max(0, (drawH - size) / 2);
    const clampedX = Math.max(-maxOffsetX, Math.min(maxOffsetX, cropOffsetX));
    const clampedY = Math.max(-maxOffsetY, Math.min(maxOffsetY, cropOffsetY));

    const dx = (size - drawW) / 2 + clampedX;
    const dy = (size - drawH) / 2 + clampedY;

    ctx.fillStyle = "#0F1B15";
    ctx.fillRect(0, 0, size, size);
    ctx.drawImage(cropImageEl, dx, dy, drawW, drawH);
  }

  $effect(() => {
    // Re-render crop preview whenever zoom, pan offset, or image changes
    if (cropDataUrl && cropImageEl && cropCanvasEl) {
      void cropZoom;
      void cropOffsetX;
      void cropOffsetY;
      drawCropPreview();
    }
  });

  function handleFileUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    if (!file.type.startsWith("image/")) {
      profileMsg = { type: "error", text: "Please choose an image file (PNG, JPG, or WebP)." };
      input.value = "";
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = reader.result as string;
      const img = new Image();
      img.onload = () => {
        cropImageEl = img;
        cropZoom = 1.15;
        cropOffsetX = 0;
        cropOffsetY = 0;
        cropDataUrl = dataUrl;
        profileMsg = null;
        requestAnimationFrame(() => drawCropPreview());
      };
      img.src = dataUrl;
    };
    reader.readAsDataURL(file);
    input.value = "";
  }

  function handleCropPointerDown(e: PointerEvent) {
    isDraggingCrop = true;
    dragStartX = e.clientX - cropOffsetX;
    dragStartY = e.clientY - cropOffsetY;
    (e.currentTarget as HTMLElement)?.setPointerCapture?.(e.pointerId);
  }

  function handleCropPointerMove(e: PointerEvent) {
    if (!isDraggingCrop || !cropImageEl) return;
    const size = 256;
    const imgW = cropImageEl.naturalWidth || 1;
    const imgH = cropImageEl.naturalHeight || 1;
    const baseScale = Math.max(size / imgW, size / imgH);
    const drawW = imgW * baseScale * cropZoom;
    const drawH = imgH * baseScale * cropZoom;
    const maxOffsetX = Math.max(0, (drawW - size) / 2);
    const maxOffsetY = Math.max(0, (drawH - size) / 2);

    cropOffsetX = Math.max(-maxOffsetX, Math.min(maxOffsetX, e.clientX - dragStartX));
    cropOffsetY = Math.max(-maxOffsetY, Math.min(maxOffsetY, e.clientY - dragStartY));
  }

  function handleCropPointerUp() {
    isDraggingCrop = false;
  }

  function applyCroppedPhoto() {
    if (!cropCanvasEl) return;
    drawCropPreview();
    avatarUrl = cropCanvasEl.toDataURL("image/jpeg", 0.9);
    cropDataUrl = null;
    cropImageEl = null;
    profileMsg = {
      type: "success",
      text: "Photo adjusted! Click 'Save Profile Changes' below to save your new picture."
    };
  }

  function cancelCropPhoto() {
    cropDataUrl = null;
    cropImageEl = null;
  }

  async function handleSaveProfile(e: SubmitEvent) {
    e.preventDefault();
    profileMsg = null;
    const trimmedName = displayName.trim();
    if (!trimmedName) {
      profileMsg = { type: "error", text: "Display name cannot be empty." };
      return;
    }

    // If user is still in crop mode without clicking Apply, apply it automatically
    if (cropDataUrl && cropCanvasEl) {
      drawCropPreview();
      avatarUrl = cropCanvasEl.toDataURL("image/jpeg", 0.9);
      cropDataUrl = null;
      cropImageEl = null;
    }

    const finalImage = avatarUrl.trim() || DEFAULT_USER_AVATAR;
    savingProfile = true;
    try {
      const res = await authClient.updateUser({
        name: trimmedName,
        image: finalImage,
      });

      if (res?.error) {
        profileMsg = {
          type: "error",
          text: toSafeUserMessage(new Error(res.error.message), "Unable to update profile right now.")
        };
      } else {
        user = { ...user, name: trimmedName, image: finalImage };
        userStore.patch({ name: trimmedName, image: finalImage });
        profileMsg = { type: "success", text: "Your photo and display name have been saved." };
      }
    } catch (err: any) {
      profileMsg = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to update profile right now.")
      };
    } finally {
      savingProfile = false;
    }
  }

  async function handleSendVerificationEmail() {
    if (!user?.email) return;
    sendingVerifyEmail = true;
    profileMsg = null;
    try {
      await authClient.sendVerificationEmail({
        email: user.email,
        callbackURL: `${window.location.origin}/app/profile`,
      });
      profileMsg = {
        type: "success",
        text: "Verification link sent! Check your inbox to confirm your email."
      };
    } catch {
      profileMsg = {
        type: "error",
        text: "Unable to send verification email right now."
      };
    } finally {
      sendingVerifyEmail = false;
    }
  }

  async function handleChangePassword(e: SubmitEvent) {
    e.preventDefault();
    passwordMsg = null;

    if (!currentPassword || !newPassword) {
      passwordMsg = { type: "error", text: "Please fill in both your current and new passwords." };
      return;
    }
    if (newPassword.length < 8) {
      passwordMsg = { type: "error", text: "New password must be at least 8 characters long." };
      return;
    }
    if (newPassword !== confirmPassword) {
      passwordMsg = { type: "error", text: "New password and confirmation do not match." };
      return;
    }

    savingPassword = true;
    try {
      const res = await authClient.changePassword({
        currentPassword,
        newPassword,
        revokeOtherSessions: revokeOthersOnPwd,
      });

      if (res?.error) {
        passwordMsg = {
          type: "error",
          text: "Current password is incorrect or new password does not meet security requirements."
        };
      } else {
        currentPassword = "";
        newPassword = "";
        confirmPassword = "";
        passwordMsg = { type: "success", text: "Your password has been updated." };
        await loadUserAndSessions();
      }
    } catch (err: any) {
      passwordMsg = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to change password right now.")
      };
    } finally {
      savingPassword = false;
    }
  }

  async function handleEnable2FA(e: SubmitEvent) {
    e.preventDefault();
    twoFactorMsg = null;
    if (!twoFactorPassword) {
      twoFactorMsg = { type: "error", text: "Enter your current password to set up 2-Step Verification." };
      return;
    }

    twoFactorLoading = true;
    try {
      const res = await authClient.twoFactor.enable({
        password: twoFactorPassword,
      });
      if (res?.error) {
        twoFactorMsg = { type: "error", text: "Incorrect password. Unable to start 2-Step Verification." };
      } else if (res?.data && "totpURI" in res.data) {
        totpSetupUri = res.data.totpURI || "";
        totpBackupCodes = res.data.backupCodes || [];
        twoFactorPassword = "";
        twoFactorMsg = {
          type: "success",
          text: "Add the setup key below to your authenticator app, then enter the 6-digit code to finish."
        };
      }
    } catch {
      twoFactorMsg = { type: "error", text: "Unable to start 2-Step Verification right now." };
    } finally {
      twoFactorLoading = false;
    }
  }

  async function handleVerify2FASetup() {
    if (!totpVerifyCode.trim()) return;
    twoFactorLoading = true;
    twoFactorMsg = null;
    try {
      const res = await authClient.twoFactor.verifyTotp({
        code: totpVerifyCode.trim(),
      });
      if (res?.error) {
        twoFactorMsg = { type: "error", text: "Invalid 6-digit code. Please check your authenticator app and try again." };
      } else {
        user = { ...user, twoFactorEnabled: true };
        userStore.patch({ twoFactorEnabled: true });
        totpSetupUri = "";
        totpVerifyCode = "";
        twoFactorMsg = { type: "success", text: "2-Step Verification is now active on your account!" };
      }
    } catch {
      twoFactorMsg = { type: "error", text: "Unable to verify the 6-digit code." };
    } finally {
      twoFactorLoading = false;
    }
  }

  async function handleDisable2FA(e: SubmitEvent) {
    e.preventDefault();
    twoFactorMsg = null;
    if (!twoFactorPassword) {
      twoFactorMsg = { type: "error", text: "Enter your password to turn off 2-Step Verification." };
      return;
    }
    twoFactorLoading = true;
    try {
      const res = await authClient.twoFactor.disable({
        password: twoFactorPassword,
      });
      if (res?.error) {
        twoFactorMsg = { type: "error", text: "Incorrect password. Could not turn off 2-Step Verification." };
      } else {
        user = { ...user, twoFactorEnabled: false };
        userStore.patch({ twoFactorEnabled: false });
        twoFactorPassword = "";
        totpSetupUri = "";
        totpBackupCodes = [];
        twoFactorMsg = { type: "success", text: "2-Step Verification has been turned off." };
      }
    } catch {
      twoFactorMsg = { type: "error", text: "Unable to turn off 2-Step Verification right now." };
    } finally {
      twoFactorLoading = false;
    }
  }

  async function handleRevokeSession(token: string) {
    revokingToken = token;
    try {
      await authClient.revokeSession({ token });
      sessions = sessions.filter((s) => s.token !== token);
    } catch (e) {
      console.error("Failed to revoke session", e);
    } finally {
      revokingToken = null;
    }
  }
</script>

<svelte:head>
  <title>Profile & Security — Kinetiq</title>
</svelte:head>

<div class="space-y-6 w-full">
  <!-- Header -->
  <div>
    <div class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[11px] font-semibold mb-2">
      <Sparkles class="w-3.5 h-3.5" />
      Your Account
    </div>
    <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
      Profile & Security
    </h1>
    <p class="text-xs sm:text-sm text-[var(--text-secondary)] mt-1">
      Update your profile photo, display name, password, 2-Step Verification, and signed-in devices.
    </p>
  </div>

  {#if loading}
    <div class="p-16 text-center text-xs font-mono text-[var(--text-muted)]">
      LOADING YOUR PROFILE...
    </div>
  {:else if user}
    <!-- Balanced 12-Column Bento Layout -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
      <!-- LEFT COLUMN (8 cols): Profile Identity, Change Password, and 2-Step Verification -->
      <div class="lg:col-span-8 space-y-6">
        <!-- 1. PROFILE PHOTO & PERSONAL IDENTITY CARD -->
        <Card>
          <form onsubmit={handleSaveProfile} class="space-y-6">
            <div class="pb-4 border-b border-[var(--border-subtle)] flex items-center justify-between">
              <div>
                <h2 class="text-base font-bold text-[var(--text-primary)] flex items-center gap-2">
                  <User class="w-4 h-4 text-[var(--brand-primary)]" />
                  Profile Photo & Name
                </h2>
                <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                  Personalize how your name and photo appear across your workspace.
                </p>
              </div>
              <div class="flex items-center gap-2">
                {#if user.emailVerified}
                  <Badge variant="emerald">Email Verified</Badge>
                {:else}
                  <button
                    type="button"
                    onclick={handleSendVerificationEmail}
                    disabled={sendingVerifyEmail}
                    class="px-3 py-1 rounded-full text-[11px] font-semibold bg-amber-500/15 text-amber-700 dark:text-amber-300 hover:bg-amber-500/25 transition-colors cursor-pointer"
                  >
                    {sendingVerifyEmail ? "Sending..." : "Verify Email"}
                  </button>
                {/if}
              </div>
            </div>

            {#if profileMsg}
              <div
                class="p-3.5 rounded-2xl text-xs font-medium flex items-center gap-2 {profileMsg.type === 'success' ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
              >
                {#if profileMsg.type === "success"}
                  <Check class="w-4 h-4 shrink-0" />
                {:else}
                  <AlertTriangle class="w-4 h-4 shrink-0" />
                {/if}
                <span>{profileMsg.text}</span>
              </div>
            {/if}

            <!-- Interactive Pre-Upload Image Crop State (shown when user picks a file) -->
            {#if cropDataUrl}
              <div class="p-5 rounded-2xl bg-[var(--bg-surface-muted)] border-2 border-[var(--brand-primary)]/40 space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <div class="text-sm font-bold text-[var(--text-primary)] flex items-center gap-1.5">
                      <Move class="w-4 h-4 text-[var(--brand-primary)]" />
                      Crop & Adjust Your Photo
                    </div>
                    <p class="text-xs text-[var(--text-secondary)]">
                      Drag the photo to reposition and use the slider to zoom in or out before applying.
                    </p>
                  </div>
                  <button
                    type="button"
                    onclick={cancelCropPhoto}
                    class="p-1.5 rounded-full text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface)] transition-colors cursor-pointer"
                    title="Cancel crop"
                  >
                    <X class="w-4 h-4" />
                  </button>
                </div>

                <div class="flex flex-col sm:flex-row items-center gap-6">
                  <!-- Interactive 176x176 Circular Crop Viewport backed by 256x256 Canvas -->
                  <div
                    class="relative w-44 h-44 rounded-full overflow-hidden border-2 border-[var(--brand-primary)] shadow-md cursor-grab active:cursor-grabbing select-none shrink-0 touch-none bg-[#0F1B15]"
                    onpointerdown={handleCropPointerDown}
                    onpointermove={handleCropPointerMove}
                    onpointerup={handleCropPointerUp}
                    onpointercancel={handleCropPointerUp}
                    role="slider"
                    tabindex="0"
                    aria-label="Drag to reposition profile photo"
                    aria-valuenow={Math.round(cropZoom * 100)}
                  >
                    <canvas
                      bind:this={cropCanvasEl}
                      width={256}
                      height={256}
                      class="w-full h-full object-cover pointer-events-none"
                    ></canvas>
                    <div class="absolute inset-0 rounded-full ring-1 ring-inset ring-white/20 pointer-events-none"></div>
                  </div>

                  <!-- Zoom Controls & Action Buttons -->
                  <div class="flex-1 w-full space-y-4">
                    <div class="space-y-2">
                      <div class="flex items-center justify-between text-xs">
                        <span class="font-semibold text-[var(--text-secondary)] flex items-center gap-1.5">
                          <ZoomIn class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                          Zoom Level
                        </span>
                        <span class="font-mono font-bold text-[var(--text-primary)]">
                          {Math.round(cropZoom * 100)}%
                        </span>
                      </div>
                      <input
                        type="range"
                        min="1"
                        max="3"
                        step="0.02"
                        bind:value={cropZoom}
                        class="w-full accent-[var(--brand-primary)] cursor-pointer"
                      />
                    </div>

                    <div class="flex flex-wrap items-center gap-2.5 pt-1">
                      <Button type="button" variant="primary" size="sm" onclick={applyCroppedPhoto}>
                        <Check class="w-3.5 h-3.5" />
                        Apply Crop
                      </Button>
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onclick={() => {
                          cropZoom = 1.15;
                          cropOffsetX = 0;
                          cropOffsetY = 0;
                        }}
                      >
                        Reset Position
                      </Button>
                      <button
                        type="button"
                        onclick={cancelCropPhoto}
                        class="px-3 py-1.5 rounded-full text-xs font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer transition-colors"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            {:else}
              <!-- Standard Avatar Preview & Upload Controls (No Extra Preset Buttons) -->
              <div class="flex flex-col sm:flex-row items-start sm:items-center gap-6 p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)]">
                <div class="relative group shrink-0">
                  <img
                    src={previewAvatarSrc}
                    alt={displayName || "Profile Avatar"}
                    class="w-20 h-20 rounded-full object-cover border-2 border-[var(--brand-primary)] shadow-xs"
                  />
                  <label
                    for="avatarFileInput"
                    class="absolute -bottom-1 -right-1 w-8 h-8 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] shadow-xs flex items-center justify-center text-[var(--brand-primary)] hover:scale-105 transition-transform cursor-pointer"
                    title="Upload custom photo"
                  >
                    <Camera class="w-4 h-4" />
                  </label>
                </div>

                <div class="space-y-3 flex-1">
                  <div>
                    <div class="text-sm font-bold text-[var(--text-primary)]">Profile Picture</div>
                    <p class="text-xs text-[var(--text-secondary)]">
                      Upload a photo from your device (PNG, JPG, or WebP). You can crop and zoom before saving.
                    </p>
                  </div>

                  <div class="flex flex-wrap items-center gap-2.5">
                    <label
                      for="avatarFileInput"
                      class="inline-flex items-center gap-1.5 px-4 py-1.5 rounded-full text-xs font-semibold bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[var(--text-primary)] hover:border-[var(--brand-primary)] transition-colors cursor-pointer"
                    >
                      <Upload class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                      Upload Photo
                    </label>

                    {#if avatarUrl && avatarUrl !== DEFAULT_USER_AVATAR}
                      <button
                        type="button"
                        onclick={() => (avatarUrl = DEFAULT_USER_AVATAR)}
                        class="px-3.5 py-1.5 rounded-full text-xs font-medium text-[var(--text-muted)] hover:text-rose-600 cursor-pointer transition-colors"
                      >
                        Remove Custom Photo
                      </button>
                    {/if}
                  </div>
                </div>
              </div>
            {/if}

            <input
              id="avatarFileInput"
              type="file"
              accept="image/png,image/jpeg,image/webp"
              onchange={handleFileUpload}
              class="hidden"
            />

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
              <div class="space-y-1.5">
                <label for="displayNameInput" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Display Name
                </label>
                <input
                  id="displayNameInput"
                  type="text"
                  bind:value={displayName}
                  placeholder="Your Name"
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-medium text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  required
                />
              </div>

              <div class="space-y-1.5">
                <label for="emailReadonly" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Email Address
                </label>
                <input
                  id="emailReadonly"
                  type="email"
                  value={user.email}
                  readonly
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)]/60 border border-[var(--border-subtle)] text-sm font-mono text-[var(--text-muted)] cursor-not-allowed"
                />
              </div>
            </div>

            <div class="flex justify-end">
              <Button type="submit" variant="primary" size="md" loading={savingProfile}>
                Save Profile Changes
              </Button>
            </div>
          </form>
        </Card>

        <!-- 2. CHANGE PASSWORD CARD -->
        <Card>
          <form onsubmit={handleChangePassword} class="space-y-5">
            <div class="pb-4 border-b border-[var(--border-subtle)]">
              <h2 class="text-base font-bold text-[var(--text-primary)] flex items-center gap-2">
                <Lock class="w-4 h-4 text-[var(--brand-primary)]" />
                Change Password
              </h2>
              <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                Choose a strong password with at least 8 characters to keep your account safe.
              </p>
            </div>

            {#if passwordMsg}
              <div
                class="p-3.5 rounded-2xl text-xs font-medium flex items-center gap-2 {passwordMsg.type === 'success' ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
              >
                {#if passwordMsg.type === "success"}
                  <Check class="w-4 h-4 shrink-0" />
                {:else}
                  <AlertTriangle class="w-4 h-4 shrink-0" />
                {/if}
                <span>{passwordMsg.text}</span>
              </div>
            {/if}

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div class="space-y-1.5">
                <label for="currentPwd" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Current Password
                </label>
                <input
                  id="currentPwd"
                  type="password"
                  bind:value={currentPassword}
                  placeholder="••••••••"
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  required
                />
              </div>

              <div class="space-y-1.5">
                <label for="newPwd" class="text-xs font-semibold text-[var(--text-secondary)]">
                  New Password (8+ characters)
                </label>
                <input
                  id="newPwd"
                  type="password"
                  bind:value={newPassword}
                  placeholder="••••••••"
                  minlength={8}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  required
                />
              </div>

              <div class="space-y-1.5">
                <label for="confirmPwd" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Confirm New Password
                </label>
                <input
                  id="confirmPwd"
                  type="password"
                  bind:value={confirmPassword}
                  placeholder="••••••••"
                  minlength={8}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  required
                />
              </div>
            </div>

            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pt-2">
              <label class="inline-flex items-center gap-2 text-xs text-[var(--text-secondary)] cursor-pointer">
                <input
                  type="checkbox"
                  bind:checked={revokeOthersOnPwd}
                  class="w-4 h-4 accent-[var(--brand-primary)] rounded"
                />
                <span>Sign out of all other devices after changing password</span>
              </label>

              <Button type="submit" variant="primary" size="md" loading={savingPassword}>
                Update Password
              </Button>
            </div>
          </form>
        </Card>

        <!-- 3. TWO-STEP VERIFICATION (2FA) CARD -->
        <Card class="space-y-5">
          <div class="pb-4 border-b border-[var(--border-subtle)] flex items-center justify-between">
            <div>
              <h2 class="text-base font-bold text-[var(--text-primary)] flex items-center gap-2">
                <Smartphone class="w-4 h-4 text-[var(--brand-primary)]" />
                2-Step Verification (2FA)
              </h2>
              <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                Add an extra layer of security using Google Authenticator, 1Password, or Authy.
              </p>
            </div>
            {#if user.twoFactorEnabled}
              <Badge variant="emerald" pulse={true}>Active</Badge>
            {:else}
              <Badge variant="neutral">Off</Badge>
            {/if}
          </div>

          {#if twoFactorMsg}
            <div
              class="p-3.5 rounded-2xl text-xs font-medium flex items-center gap-2 {twoFactorMsg.type === 'success' ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
            >
              {#if twoFactorMsg.type === "success"}
                <Check class="w-4 h-4 shrink-0" />
              {:else}
                <AlertTriangle class="w-4 h-4 shrink-0" />
              {/if}
              <span>{twoFactorMsg.text}</span>
            </div>
          {/if}

          {#if totpSetupUri}
            <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] space-y-4">
              <div>
                <div class="text-xs font-bold text-[var(--text-primary)]">1. Connect Your Authenticator App</div>
                <p class="text-[11px] text-[var(--text-secondary)] mt-0.5">
                  Copy and add this setup link into your authenticator app:
                </p>
                <code class="block mt-2 p-2.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[11px] font-mono text-[var(--brand-primary)] break-all select-all">
                  {totpSetupUri}
                </code>
              </div>

              {#if totpBackupCodes.length > 0}
                <div>
                  <div class="text-xs font-bold text-[var(--text-primary)]">2. Save Your Backup Recovery Codes</div>
                  <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 mt-2">
                    {#each totpBackupCodes as code}
                      <code class="px-2.5 py-1 rounded-lg bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[11px] font-mono text-center text-[var(--text-primary)]">
                        {code}
                      </code>
                    {/each}
                  </div>
                </div>
              {/if}

              <div class="flex items-center gap-3 pt-2">
                <input
                  type="text"
                  inputmode="numeric"
                  maxlength={6}
                  bind:value={totpVerifyCode}
                  placeholder="Enter 6-digit code"
                  class="px-4 py-2 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-sm font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                />
                <Button variant="primary" size="md" onclick={handleVerify2FASetup} loading={twoFactorLoading}>
                  Verify & Turn On
                </Button>
              </div>
            </div>
          {:else if !user.twoFactorEnabled}
            <form onsubmit={handleEnable2FA} class="flex flex-col sm:flex-row items-stretch sm:items-end gap-4">
              <div class="flex-1 space-y-1.5">
                <label for="enable2faPwd" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Enter Your Password to Set Up 2-Step Verification
                </label>
                <input
                  id="enable2faPwd"
                  type="password"
                  bind:value={twoFactorPassword}
                  placeholder="••••••••"
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  required
                />
              </div>
              <Button type="submit" variant="secondary" size="md" loading={twoFactorLoading}>
                Set Up 2FA
              </Button>
            </form>
          {:else}
            <form onsubmit={handleDisable2FA} class="flex flex-col sm:flex-row items-stretch sm:items-end gap-4">
              <div class="flex-1 space-y-1.5">
                <label for="disable2faPwd" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Enter Your Password to Turn Off 2-Step Verification
                </label>
                <input
                  id="disable2faPwd"
                  type="password"
                  bind:value={twoFactorPassword}
                  placeholder="••••••••"
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:border-rose-500"
                  required
                />
              </div>
              <Button type="submit" variant="danger" size="md" loading={twoFactorLoading}>
                Turn Off 2FA
              </Button>
            </form>
          {/if}
        </Card>
      </div>

      <!-- RIGHT COLUMN (4 cols): Live Identity Card Preview & Signed-In Devices -->
      <div class="lg:col-span-4 space-y-6">
        <!-- Live Identity Card Preview -->
        <div class="rounded-[24px] bg-gradient-to-br from-[#127852] via-[#106B49] to-[#0C5439] text-white p-6 shadow-[var(--shadow-bento)] space-y-5 relative overflow-hidden">
          <div class="w-32 h-32 rounded-full bg-white/5 absolute -right-10 -top-10 pointer-events-none"></div>

          <div class="flex items-center gap-3.5 relative z-10">
            <img
              src={previewAvatarSrc}
              alt={displayName || "Avatar"}
              class="w-14 h-14 rounded-full object-cover border-2 border-white/30 shrink-0"
            />
            <div class="min-w-0">
              <span class="inline-block text-[10px] font-mono uppercase tracking-widest px-2.5 py-0.5 rounded-full bg-white/15 text-emerald-100 mb-1">
                {user.role || "Workspace Owner"}
              </span>
              <h3 class="text-lg font-extrabold truncate">
                {displayName || user.name || "Member"}
              </h3>
              <p class="text-xs text-emerald-100/80 truncate font-mono">{user.email}</p>
            </div>
          </div>

          <div class="pt-3 border-t border-white/15 grid grid-cols-2 gap-3 text-xs relative z-10">
            <div class="p-3 rounded-2xl bg-white/10">
              <div class="text-[10px] text-emerald-100/75 flex items-center gap-1">
                <Mail class="w-3 h-3" />
                2FA Protection
              </div>
              <div class="font-bold mt-0.5">{user.twoFactorEnabled ? "Protected" : "Standard"}</div>
            </div>
            <div class="p-3 rounded-2xl bg-white/10">
              <div class="text-[10px] text-emerald-100/75 flex items-center gap-1">
                <KeyRound class="w-3 h-3" />
                Signed-In Devices
              </div>
              <div class="font-bold mt-0.5">{sessions.length || 1} Device(s)</div>
            </div>
          </div>
        </div>

        <!-- Active Devices & Sessions Card -->
        <Card class="space-y-4">
          <div class="pb-3 border-b border-[var(--border-subtle)] flex items-center justify-between">
            <div>
              <h2 class="text-sm font-bold text-[var(--text-primary)] flex items-center gap-2">
                <ShieldCheck class="w-4 h-4 text-[var(--brand-primary)]" />
                Signed-In Devices
              </h2>
              <p class="text-[11px] text-[var(--text-secondary)] mt-0.5">
                Browsers currently logged into your account
              </p>
            </div>
            <Badge variant="neutral">{sessions.length || 1} active</Badge>
          </div>

          {#if sessions.length === 0}
            <div class="py-4 text-xs text-[var(--text-secondary)] flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
                  <Monitor class="w-4 h-4" />
                </div>
                <div>
                  <div class="font-bold text-[var(--text-primary)]">This Web Browser</div>
                  <div class="text-[11px] font-mono text-[var(--text-muted)]">Active now</div>
                </div>
              </div>
              <Badge variant="emerald" pulse={true}>This Device</Badge>
            </div>
          {:else}
            <div class="divide-y divide-[var(--border-subtle)]">
              {#each sessions as s (s.id || s.token)}
                {@const isCurrent = s.token === currentSessionToken}
                <div class="py-3 first:pt-0 last:pb-0 flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2.5 min-w-0">
                    <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center shrink-0">
                      <Monitor class="w-4 h-4" />
                    </div>
                    <div class="min-w-0">
                      <div class="flex items-center gap-1.5">
                        <span class="text-xs font-bold text-[var(--text-primary)] truncate">
                          {s.userAgent ? s.userAgent.split(" ")[0] : "Web Browser"}
                        </span>
                        {#if isCurrent}
                          <Badge variant="emerald" pulse={true}>This Device</Badge>
                        {/if}
                      </div>
                      <div class="text-[10px] font-mono text-[var(--text-muted)] truncate">
                        {s.ipAddress || "127.0.0.1"} · {new Date(s.createdAt).toLocaleDateString()}
                      </div>
                    </div>
                  </div>

                  {#if !isCurrent}
                    <Button
                      variant="danger"
                      size="sm"
                      loading={revokingToken === s.token}
                      onclick={() => handleRevokeSession(s.token)}
                    >
                      <Trash2 class="w-3 h-3" />
                    </Button>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        </Card>
      </div>
    </div>
  {/if}
</div>
