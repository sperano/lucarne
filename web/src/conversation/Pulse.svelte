<script lang="ts">
  // Three dots pulsing in turn: shown while work is still under way.
</script>

<span class="dots" aria-hidden="true"><span></span><span></span><span></span></span>

<style>
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
