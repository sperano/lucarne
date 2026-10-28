<script lang="ts">
  import type { CoverageState, Limits } from '../lib/contract';
  import type { Conversation, Item } from '../lib/conversation';
  import { unreachable } from '../lib/unreachable';
  import EditView from './EditView.svelte';
  import ErrorNotice from './ErrorNotice.svelte';
  import GapNotice from './GapNotice.svelte';
  import LifecycleLine from './LifecycleLine.svelte';
  import MessageView from './MessageView.svelte';
  import OrphanResult from './OrphanResult.svelte';
  import RawLine from './RawLine.svelte';
  import ThinkingLine from './ThinkingLine.svelte';
  import ToolCard from './ToolCard.svelte';
  import UsageLine from './UsageLine.svelte';

  let {
    item,
    conversation,
    coverage,
    limits,
    targeted,
    ongoing = false,
  }: {
    item: Item;
    conversation: Conversation;
    coverage: CoverageState;
    limits: Limits;
    /** Whether the page's #event= link, or the search result shown, points into this item: it opens. */
    targeted: boolean;
    /** Whether the item is the end of an attempt still being read: thinking shows as under way. */
    ongoing?: boolean;
  } = $props();

  function parentName(parentCallId: string | undefined): string | undefined {
    if (parentCallId === undefined) return undefined;
    return conversation.calls.get(parentCallId)?.call.toolCall.name ?? `call ${parentCallId}`;
  }
</script>

{#if item.kind === 'tool'}
  <ToolCard
    {item}
    parentName={parentName(item.call.toolCall.parentCallId)}
    {coverage}
    {limits}
    open={targeted}
  />
{:else if item.event.kind === 'message'}
  <MessageView event={item.event} {limits} open={targeted} />
{:else if item.event.kind === 'tool_result'}
  <OrphanResult
    event={item.event}
    repeated={conversation.calls.has(item.event.toolResult.callId)}
    {limits}
  />
{:else if item.event.kind === 'edit'}
  <EditView event={item.event} {limits} />
{:else if item.event.kind === 'usage'}
  <UsageLine event={item.event} />
{:else if item.event.kind === 'error'}
  <ErrorNotice event={item.event} {limits} />
{:else if item.event.kind === 'lifecycle' && item.event.lifecycle.phase === 'thinking'}
  <ThinkingLine {ongoing} />
{:else if item.event.kind === 'lifecycle'}
  <LifecycleLine event={item.event} {limits} />
{:else if item.event.kind === 'raw'}
  <RawLine event={item.event} {limits} open={targeted} />
  <!-- eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- the last kind is tested so that unreachable() fails to compile when a kind is added. -->
{:else if item.event.kind === 'gap'}
  <GapNotice event={item.event} />
{:else}
  {unreachable(item.event)}
{/if}
