<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    code: string;
    label?: string;
    children?: Snippet;
  }

  let { code, label = "", children }: Props = $props();
  let copied = $state(false);
  let failed = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      copied = true;
      failed = false;
    } catch {
      failed = true;
    }
    clearTimeout(timer);
    timer = setTimeout(() => {
      copied = false;
      failed = false;
    }, 2000);
  }
</script>

<div class="block">
  {#if label}
    <span class="label">{label}</span>
  {/if}
  <div class="code">
    {#if children}
      {@render children()}
    {:else}
      <pre><code>{code}</code></pre>
    {/if}
  </div>
  <button onclick={copy} aria-label="Copy to clipboard">
    {copied ? "Copied" : failed ? "Failed" : "Copy"}
  </button>
</div>

<style>
  .block {
    position: relative;
  }

  .label {
    display: block;
    font-family: var(--mono);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
    margin-bottom: 0.5rem;
  }

  /* Shiki emits both themes as CSS vars; pick one per colour scheme. */
  .code :global(pre.shiki) {
    background: var(--shiki-light-bg) !important;
    color: var(--shiki-light) !important;
  }

  .code :global(pre.shiki span) {
    color: var(--shiki-light);
  }

  @media (prefers-color-scheme: dark) {
    .code :global(pre.shiki) {
      background: var(--shiki-dark-bg) !important;
      color: var(--shiki-dark) !important;
    }

    .code :global(pre.shiki span) {
      color: var(--shiki-dark);
    }
  }

  /* Opaque, and themed to the code block it sits on: shiki gives the block a
     light background in light mode, so a light-on-light label vanished. */
  button {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    font-family: var(--mono);
    font-size: 0.72rem;
    padding: 0.3rem 0.6rem;
    color: #3c3833;
    background: #f0ece5;
    border: 1px solid #d4cdc2;
    border-radius: 5px;
    cursor: pointer;
    opacity: 0;
    transition:
      opacity 0.15s ease,
      background-color 0.15s ease;
  }

  @media (prefers-color-scheme: dark) {
    button {
      color: #e4e0d8;
      background: #2c3239;
      border-color: #454c55;
    }
  }

  .block:hover button,
  button:focus-visible {
    opacity: 1;
  }

  button:hover {
    background: #e2dcd2;
  }

  @media (prefers-color-scheme: dark) {
    button:hover {
      background: #3a424b;
    }
  }

  /* Touch devices never hover, so the affordance must stay visible. */
  @media (hover: none) {
    button {
      opacity: 1;
    }
  }
</style>
