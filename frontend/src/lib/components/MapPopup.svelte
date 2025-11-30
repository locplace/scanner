<script lang="ts">
	interface Props {
		fqdn: string;
		rootDomain: string;
		latitude: number;
		longitude: number;
		altitudeM: number;
		rawRecord: string;
	}

	let { fqdn, rootDomain, latitude, longitude, altitudeM, rawRecord }: Props = $props();
</script>

<div class="popup">
	<div class="popup-title">{fqdn}</div>
	<div class="popup-domain">{rootDomain}</div>
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
	}

	.popup-title {
		font-weight: 600;
		font-size: 15px;
		margin-bottom: 4px;
		word-break: break-all;
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
