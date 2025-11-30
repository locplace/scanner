<script lang="ts">
	interface Props {
		fqdns: string[];
		rootDomains: string[];
		latitude: number;
		longitude: number;
		altitudeM: number;
		rawRecord: string;
	}

	let { fqdns, rootDomains, latitude, longitude, altitudeM, rawRecord }: Props = $props();
</script>

<div class="popup">
	{#if fqdns.length === 1}
		<div class="popup-title">{fqdns[0]}</div>
		<div class="popup-domain">{rootDomains[0]}</div>
	{:else}
		<div class="popup-header">{fqdns.length} records at this location</div>
		<ul class="popup-list">
			{#each fqdns as fqdn}
				<li>{fqdn}</li>
			{/each}
		</ul>
		<div class="popup-domain">{rootDomains.join(', ')}</div>
	{/if}
	<div class="popup-coords">
		{latitude.toFixed(6)}, {longitude.toFixed(6)}<br />
		Altitude: {altitudeM}m
	</div>
	<div class="popup-raw">{rawRecord}</div>
</div>

<style>
	.popup {
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
		font-size: 14px;
		line-height: 1.4;
		max-width: 300px;
	}

	.popup-title {
		font-weight: 600;
		font-size: 15px;
		margin-bottom: 4px;
		word-break: break-all;
	}

	.popup-header {
		font-weight: 600;
		font-size: 13px;
		color: #666;
		margin-bottom: 8px;
		padding-bottom: 6px;
		border-bottom: 1px solid #eee;
	}

	.popup-list {
		list-style: none;
		margin: 0 0 8px 0;
		padding: 0;
		max-height: 150px;
		overflow-y: auto;
	}

	.popup-list li {
		padding: 2px 0;
		word-break: break-all;
		font-weight: 500;
	}

	.popup-domain {
		color: #666;
		font-size: 13px;
		margin-bottom: 8px;
	}

	.popup-coords {
		font-size: 12px;
		color: #444;
		margin-bottom: 8px;
	}

	.popup-raw {
		font-family: monospace;
		font-size: 11px;
		background: #f5f5f5;
		padding: 6px 8px;
		border-radius: 4px;
		word-break: break-all;
	}
</style>
