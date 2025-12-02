<script lang="ts">
	import { onMount, mount } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import MapPopup from '$lib/components/MapPopup.svelte';

	let mapContainer: HTMLDivElement;
	let map: maplibregl.Map;

	// Panel states
	let isAboutOpen = true;
	let isStatsOpen = false;
	let isSearchOpen = false;
	let isDarkTheme = false;

	// Stats
	let stats: {
		total_loc_records: number;
		unique_root_domains_with_loc: number;
	} | null = null;

	// Search state
	let searchQuery = '';
	let searchTimeout: ReturnType<typeof setTimeout>;
	let fullGeoJSON: GeoJSON.FeatureCollection | null = null;

	interface FQDNEntry {
		fqdn: string;
		feature: GeoJSON.Feature;
		lastSeenAt: Date;
	}
	interface LocationEntry {
		rootDomain: string;
		feature: GeoJSON.Feature;
		lastSeenAt: Date;
		fqdnCount: number;
	}
	let fqdnIndex: FQDNEntry[] = [];
	let locationIndex: LocationEntry[] = [];
	let displayedEntries: (FQDNEntry | LocationEntry)[] = [];

	async function loadStats() {
		try {
			const response = await fetch('/api/public/stats');
			if (response.ok) {
				stats = await response.json();
			}
		} catch (e) {
			console.error('Failed to load stats:', e);
		}
	}

	function toggleSearch() {
		isSearchOpen = !isSearchOpen;
	}

	function buildFQDNIndex(geojson: GeoJSON.FeatureCollection): FQDNEntry[] {
		const entries: FQDNEntry[] = [];
		for (const feature of geojson.features) {
			const props = feature.properties;
			const fqdns = typeof props?.fqdns === 'string' ? JSON.parse(props.fqdns) : props?.fqdns || [];
			const lastSeenAt = props?.last_seen_at ? new Date(props.last_seen_at) : new Date(0);

			for (const fqdn of fqdns) {
				entries.push({ fqdn, feature, lastSeenAt });
			}
		}
		// Sort by lastSeenAt descending (newest first)
		entries.sort((a, b) => b.lastSeenAt.getTime() - a.lastSeenAt.getTime());
		return entries;
	}

	function buildLocationIndex(geojson: GeoJSON.FeatureCollection): LocationEntry[] {
		const entries: LocationEntry[] = [];
		for (const feature of geojson.features) {
			const props = feature.properties;
			const rootDomains = typeof props?.root_domains === 'string' ? JSON.parse(props.root_domains) : props?.root_domains || [];
			const fqdns = typeof props?.fqdns === 'string' ? JSON.parse(props.fqdns) : props?.fqdns || [];
			const lastSeenAt = props?.last_seen_at ? new Date(props.last_seen_at) : new Date(0);

			// Use first root domain as representative
			const rootDomain = rootDomains[0] || fqdns[0] || 'unknown';
			entries.push({ rootDomain, feature, lastSeenAt, fqdnCount: fqdns.length });
		}
		// Sort by lastSeenAt descending (newest first)
		entries.sort((a, b) => b.lastSeenAt.getTime() - a.lastSeenAt.getTime());
		return entries;
	}

	function handleSearchInput(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		searchQuery = value;

		// Debounce the filtering
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			applyFilter(value);
		}, 150);
	}

	function applyFilter(query: string) {
		const lowerQuery = query.toLowerCase().trim();

		if (!lowerQuery) {
			// No query: show recent locations (deduplicated by feature), all points on map
			displayedEntries = locationIndex.slice(0, 50);
			if (fullGeoJSON && map.getSource('loc-records')) {
				(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(fullGeoJSON);
			}
		} else {
			// Filter FQDNs (search matches individual FQDNs)
			const matchingEntries = fqdnIndex.filter(entry =>
				entry.fqdn.toLowerCase().includes(lowerQuery)
			);
			displayedEntries = matchingEntries.slice(0, 50);

			// Filter map to only show features with matching FQDNs
			if (fullGeoJSON && map.getSource('loc-records')) {
				const matchingFeatures = new Set(matchingEntries.map(e => e.feature));
				const filteredGeoJSON: GeoJSON.FeatureCollection = {
					type: 'FeatureCollection',
					features: fullGeoJSON.features.filter(f => matchingFeatures.has(f))
				};
				(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(filteredGeoJSON);
			}
		}
	}

	function selectEntry(entry: FQDNEntry | LocationEntry) {
		const coords = (entry.feature.geometry as GeoJSON.Point).coordinates as [number, number];
		const props = entry.feature.properties;

		// Zoom to location
		map.flyTo({
			center: coords,
			zoom: 12,
			duration: 1000
		});

		// Open popup after flying
		setTimeout(() => {
			const fqdns = typeof props?.fqdns === 'string' ? JSON.parse(props.fqdns) : props?.fqdns || [];
			const rootDomains = typeof props?.root_domains === 'string' ? JSON.parse(props.root_domains) : props?.root_domains || [];

			const container = document.createElement('div');
			mount(MapPopup, {
				target: container,
				props: {
					fqdns,
					rootDomains,
					latitude: coords[1],
					longitude: coords[0],
					altitudeM: props?.altitude_m || 0,
					rawRecord: props?.raw_record || ''
				}
			});

			// Close existing popups
			const existingPopups = document.querySelectorAll('.maplibregl-popup');
			existingPopups.forEach(p => p.remove());

			new maplibregl.Popup()
				.setLngLat(coords)
				.setDOMContent(container)
				.addTo(map);
		}, 1000);
	}

	function getStyleUrl(): string {
		const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		return `https://tiles.immich.cloud/v1/style/${isDark ? 'dark' : 'light'}.json`;
	}

	function toggleAbout() {
		isAboutOpen = !isAboutOpen;
	}

	function toggleStats() {
		isStatsOpen = !isStatsOpen;
	}

	onMount(() => {
		// Set initial theme and overlay state
		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		isDarkTheme = mediaQuery.matches;

		// Collapse about by default on small screens (< 768px)
		isAboutOpen = window.innerWidth >= 768;
		// Stats always starts collapsed

		map = new maplibregl.Map({
			container: mapContainer,
			style: getStyleUrl(),
			center: [0, 30],
			zoom: 2
		});

		map.addControl(new maplibregl.NavigationControl(), 'bottom-right');

		// Listen for theme changes
		const handleThemeChange = () => {
			isDarkTheme = mediaQuery.matches;
			map.setStyle(getStyleUrl());
			// Re-add LOC records after style change
			map.once('style.load', loadLOCRecords);
		};
		mediaQuery.addEventListener('change', handleThemeChange);

		map.on('load', async () => {
			await loadLOCRecords();
		});

		// Load stats
		loadStats();

		return () => {
			mediaQuery.removeEventListener('change', handleThemeChange);
			map?.remove();
		};
	});

	async function loadLOCRecords() {
		try {
			const response = await fetch('/api/public/records.geojson');
			if (!response.ok) throw new Error('Failed to fetch records');

			const geojson: GeoJSON.FeatureCollection = await response.json();

			// Store for filtering and build search indices
			fullGeoJSON = geojson;
			fqdnIndex = buildFQDNIndex(geojson);
			locationIndex = buildLocationIndex(geojson);
			displayedEntries = locationIndex.slice(0, 50);

			// Add or update the source
			if (map.getSource('loc-records')) {
				(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(geojson);
				return;
			}

			map.addSource('loc-records', {
				type: 'geojson',
				data: geojson
			});

			map.addLayer({
				id: 'points',
				type: 'circle',
				source: 'loc-records',
				paint: {
					'circle-radius': 8,
					'circle-color': '#e74c3c',
					'circle-stroke-width': 2,
					'circle-stroke-color': '#fff'
				}
			});

			// Click handler for points
			map.on('click', 'points', (e) => {
				if (!e.features?.length) return;

				const feature = e.features[0];
				const props = feature.properties;
				const coords = (feature.geometry as GeoJSON.Point).coordinates;

				// Parse arrays - they come as JSON strings from MapLibre
				const fqdns = typeof props?.fqdns === 'string' ? JSON.parse(props.fqdns) : props?.fqdns || [];
				const rootDomains = typeof props?.root_domains === 'string' ? JSON.parse(props.root_domains) : props?.root_domains || [];

				const container = document.createElement('div');
				mount(MapPopup, {
					target: container,
					props: {
						fqdns,
						rootDomains,
						latitude: coords[1],
						longitude: coords[0],
						altitudeM: props?.altitude_m || 0,
						rawRecord: props?.raw_record || ''
					}
				});

				new maplibregl.Popup()
					.setLngLat(coords as [number, number])
					.setDOMContent(container)
					.addTo(map);
			});

			// Change cursor on hover
			map.on('mouseenter', 'points', () => {
				map.getCanvas().style.cursor = 'pointer';
			});
			map.on('mouseleave', 'points', () => {
				map.getCanvas().style.cursor = '';
			});

			// Fit to data bounds if we have records
			if (geojson.features.length > 0) {
				const bounds = new maplibregl.LngLatBounds();
				for (const feature of geojson.features) {
					const coords = (feature.geometry as GeoJSON.Point).coordinates as [number, number];
					bounds.extend(coords);
				}
				map.fitBounds(bounds, { padding: 50, maxZoom: 10 });
			}
		} catch (error) {
			console.error('Error loading LOC records:', error);
		}
	}
</script>

<div id="map" bind:this={mapContainer}></div>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="panels-container" class:dark={isDarkTheme}>
	<div class="panel" class:collapsed={!isAboutOpen}>
		<div class="panel-header" onclick={toggleAbout}>
			<span class="title">About LOC.place</span>
			<span class="toggle-icon">{isAboutOpen ? '−' : '+'}</span>
		</div>
		<div class="panel-content">
			<p>
				As one of the old, core pieces internet infrastructure, the DNS system has many obscure and forgotten corners.
				One of those is the <a href="https://en.wikipedia.org/wiki/LOC_record">LOC record</a>, which ties a domain name
				to a set of geographical coordinates.
				There are only a few thousand of these records in the entirety of DNS, making it feasible to map all of them.
			</p>
			<p>
				This effort would not have been possible without tb0hdan's <a href="https://github.com/tb0hdan/domains/">list of domains</a>,
				or without my colleagues taking it as a personal challenge to run as many scanners as they could.
			</p>
			<p>
				You can find the source code on <a href="https://github.com/locplace/locplace">github</a>.
				If you have any questions, remarks, or you just want to say hi, don't hesitate to <a href="mailto:contact@loc.place">email me</a>.
			</p>
		</div>
	</div>

	{#if stats}
	<div class="panel" class:collapsed={!isStatsOpen}>
		<div class="panel-header" onclick={toggleStats}>
			<span class="title">Statistics</span>
			<span class="toggle-icon">{isStatsOpen ? '−' : '+'}</span>
		</div>
		<div class="panel-content stats-content">
			<div class="stat-row">
				<span class="stat-label">LOC records</span>
				<span class="stat-value">{stats.total_loc_records.toLocaleString()}</span>
			</div>
			<div class="stat-row">
				<span class="stat-label">Unique domains</span>
				<span class="stat-value">{stats.unique_root_domains_with_loc.toLocaleString()}</span>
			</div>
		</div>
	</div>
	{/if}

	<div class="panel" class:collapsed={!isSearchOpen}>
		<div class="panel-header" onclick={toggleSearch}>
			<span class="title">Search</span>
			<span class="toggle-icon">{isSearchOpen ? '−' : '+'}</span>
		</div>
		<div class="panel-content search-content">
			<div class="search-input-wrapper">
				<input
					type="text"
					placeholder="Search FQDNs..."
					value={searchQuery}
					oninput={handleSearchInput}
					class="search-input"
				/>
				{#if searchQuery}
					<button class="clear-search" onclick={() => { searchQuery = ''; applyFilter(''); }}>×</button>
				{/if}
			</div>
			<div class="fqdn-list">
				{#if displayedEntries.length === 0}
					<div class="no-results">No matching FQDNs</div>
				{:else}
					{#each displayedEntries as entry}
						<button class="fqdn-item" onclick={() => selectEntry(entry)}>
							{'fqdn' in entry ? entry.fqdn : entry.rootDomain}
						</button>
					{/each}
				{/if}
			</div>
		</div>
	</div>
</div>

<style>
	/* Prevent scroll on map page - it should fill viewport */
	:global(html), :global(body) {
		overflow: hidden;
	}

	.panels-container {
		position: absolute;
		top: 10px;
		left: 10px;
		max-width: 320px;
		width: calc(100vw - 20px);
		z-index: 1000;
		display: flex;
		flex-direction: column;
		gap: 0;
	}

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

	.panels-container.dark .panel {
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

	.panels-container.dark .panel-header {
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
		transition: max-height 0.2s ease, padding 0.2s ease, opacity 0.2s ease;
	}

	.panel.collapsed .panel-content {
		max-height: 0;
		padding-top: 0;
		padding-bottom: 0;
		opacity: 0;
	}

	.panel-content p {
		margin: 0 0 10px 0;
	}

	.panel-content p:last-child {
		margin-bottom: 0;
	}

	.panel-content a {
		color: #2563eb;
		text-decoration: none;
	}

	.panel-content a:hover {
		text-decoration: underline;
	}

	.panels-container.dark .panel-content a {
		color: #60a5fa;
	}

	/* Stats panel specific styles */
	.stats-content {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.stat-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.stat-label {
		opacity: 0.8;
	}

	.stat-value {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	@media (max-width: 400px) {
		.panels-container {
			max-width: calc(100vw - 20px);
		}
	}

	/* Search panel styles */
	.search-content {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.search-input {
		width: 100%;
		padding: 8px 10px;
		border: 1px solid rgba(0, 0, 0, 0.2);
		border-radius: 4px;
		font-size: 13px;
		background: rgba(255, 255, 255, 0.8);
		box-sizing: border-box;
	}

	.search-input:focus {
		outline: none;
		border-color: #2563eb;
	}

	.panels-container.dark .search-input {
		background: rgba(50, 50, 50, 0.8);
		border-color: rgba(255, 255, 255, 0.2);
		color: #e0e0e0;
	}

	.panels-container.dark .search-input:focus {
		border-color: #60a5fa;
	}

	.search-input-wrapper {
		position: relative;
		display: flex;
		align-items: center;
	}

	.search-input-wrapper .search-input {
		padding-right: 28px;
	}

	.clear-search {
		position: absolute;
		right: 6px;
		background: none;
		border: none;
		font-size: 16px;
		line-height: 1;
		cursor: pointer;
		color: #888;
		padding: 2px 6px;
		border-radius: 3px;
	}

	.clear-search:hover {
		background: rgba(0, 0, 0, 0.1);
		color: #444;
	}

	.panels-container.dark .clear-search {
		color: #999;
	}

	.panels-container.dark .clear-search:hover {
		background: rgba(255, 255, 255, 0.1);
		color: #ddd;
	}

	.fqdn-list {
		display: flex;
		flex-direction: column;
		max-height: 200px;
		overflow-y: auto;
	}

	.fqdn-item {
		display: block;
		width: 100%;
		padding: 6px 8px;
		text-align: left;
		background: none;
		border: none;
		border-radius: 3px;
		cursor: pointer;
		font-size: 12px;
		font-family: monospace;
		color: inherit;
		box-sizing: border-box;
	}

	.fqdn-item:hover {
		background: rgba(0, 0, 0, 0.08);
	}

	.panels-container.dark .fqdn-item:hover {
		background: rgba(255, 255, 255, 0.1);
	}

	.no-results {
		padding: 8px;
		text-align: center;
		opacity: 0.6;
		font-size: 12px;
	}
</style>
