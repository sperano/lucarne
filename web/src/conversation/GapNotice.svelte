<script lang="ts">
  import type { EventOf } from '../lib/conversation';
  import { formatCount } from '../lib/format';
  import { GAP_REASON_LABELS } from '../lib/labels';

  let { event }: { event: EventOf<'gap'> } = $props();

  const gap = $derived(event.gap);
  const count = $derived(
    gap.lines === undefined
      ? 'Lines are missing here'
      : `${formatCount(gap.lines)} ${gap.lines === 1 ? 'line is' : 'lines are'} missing here`,
  );
</script>

<p class="notice warning gap" role="note">
  <strong>{count}.</strong>
  {GAP_REASON_LABELS[gap.reason]}
  {#if gap.detail !== undefined}<span class="detail">{gap.detail}</span>{/if}
</p>
