<script lang="ts">
  let {
    label,
    text,
  }: {
    /** What the button copies, as its label says it. */
    label: string;
    /** The text copied, computed when the button is pressed. */
    text: () => string;
  } = $props();

  /** How long the outcome stays said next to the button. */
  const OUTCOME_MS = 2000;

  let outcome: 'copied' | 'refused' | undefined = $state();

  $effect(() => {
    if (outcome === undefined) return;
    const timer = setTimeout(() => (outcome = undefined), OUTCOME_MS);
    return () => {
      clearTimeout(timer);
    };
  });

  async function copy(): Promise<void> {
    try {
      // The clipboard is missing outside a secure context: the call throws, as a refusal does.
      await navigator.clipboard.writeText(text());
      outcome = 'copied';
    } catch {
      outcome = 'refused';
    }
  }
</script>

<span class="copy">
  <button type="button" onclick={copy}>{label}</button>
  <span class="outcome muted" aria-live="polite"
    >{outcome === 'copied'
      ? 'Copied'
      : outcome === 'refused'
        ? 'The browser refused to copy'
        : ''}</span
  >
</span>

<style>
  .copy {
    display: inline-flex;
    gap: 0.375rem;
    align-items: baseline;
  }
  button {
    font-size: 0.8125rem;
  }
  .outcome {
    font-size: 0.8125rem;
  }
</style>
