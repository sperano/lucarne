<script lang="ts">
  import { LIFECYCLE_LABELS } from '../lib/labels';

  // A batch of the runner's thinking progress: the model at work, animated while it still is.
  let { ongoing }: { ongoing: boolean } = $props();
</script>

<p class="thinking" class:ongoing role={ongoing ? 'status' : undefined}>
  <span class="label">{LIFECYCLE_LABELS.thinking}</span>
  {#if ongoing}
    <span class="dots" aria-hidden="true"><span></span><span></span><span></span></span>
  {/if}
</p>

<style>
  .thinking {
    margin: 0;
    color: var(--text-muted);
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }
  .ongoing .label {
    font-style: italic;
  }
  .dots {
    display: inline-flex;
    gap: 0.25rem;
  }
  .dots span {
    width: 0.375rem;
    height: 0.375rem;
    border-radius: 50%;
    background: currentColor;
    animation: pulse 1.2s ease-in-out infinite;
  }
  .dots span:nth-child(2) {
    animation-delay: 0.2s;
  }
  .dots span:nth-child(3) {
    animation-delay: 0.4s;
  }
  @keyframes pulse {
    0%,
    80%,
    100% {
      opacity: 0.25;
    }
    40% {
      opacity: 1;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .dots span {
      animation: none;
      opacity: 0.6;
    }
  }
</style>
