<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		isOpen: boolean;
		onToggle: () => void;
		children: Snippet;
		contentClass?: string;
	}

	let { title, isOpen, onToggle, children, contentClass = '' }: Props = $props();
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="panel" class:collapsed={!isOpen}>
	<div class="panel-header" onclick={onToggle}>
		<span class="title">{title}</span>
		<span class="toggle-icon">{isOpen ? '−' : '+'}</span>
	</div>
	<div class="panel-content {contentClass}">
		{@render children()}
	</div>
</div>

<style>
	.panel {
		background: rgba(255, 255, 255, 0.9);
		backdrop-filter: blur(8px);
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
		overflow: hidden;
	}

	.panel:first-child {
		border-radius: 8px 8px 0 0;
	}

	.panel:last-child {
		border-radius: 0 0 8px 8px;
	}

	.panel:only-child {
		border-radius: 8px;
	}

	:global(.panels-container.dark) .panel {
		background: rgba(30, 30, 30, 0.9);
		color: #e0e0e0;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.4);
	}

	.panel-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 10px 14px;
		cursor: pointer;
		user-select: none;
		font-weight: 600;
		font-size: 14px;
		border-bottom: 1px solid rgba(0, 0, 0, 0.1);
	}

	:global(.panels-container.dark) .panel-header {
		border-bottom-color: rgba(255, 255, 255, 0.1);
	}

	.panel.collapsed .panel-header {
		border-bottom: none;
	}

	.toggle-icon {
		font-size: 18px;
		line-height: 1;
		opacity: 0.6;
	}

	.panel-content {
		padding: 12px 14px;
		font-size: 13px;
		line-height: 1.5;
		max-height: 500px;
		overflow-y: auto;
		transition:
			max-height 0.2s ease,
			padding 0.2s ease,
			opacity 0.2s ease;
	}

	.panel.collapsed .panel-content {
		max-height: 0;
		padding-top: 0;
		padding-bottom: 0;
		opacity: 0;
	}

	/* Content link styling */
	.panel-content :global(p) {
		margin: 0 0 10px 0;
	}

	.panel-content :global(p:last-child) {
		margin-bottom: 0;
	}

	.panel-content :global(a) {
		color: #2563eb;
		text-decoration: none;
	}

	.panel-content :global(a:hover) {
		text-decoration: underline;
	}

	:global(.panels-container.dark) .panel-content :global(a) {
		color: #60a5fa;
	}
</style>
