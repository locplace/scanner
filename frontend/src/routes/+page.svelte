<script lang="ts">
	import { onMount, mount } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import MapPopup from '$lib/components/MapPopup.svelte';
	import CollapsiblePanel from '$lib/components/CollapsiblePanel.svelte';
	import type { FQDNEntry, LocationEntry, PublicStats, SearchEntry } from '$lib/types';
	import { isFQDNEntry } from '$lib/types';
	import { buildFQDNIndex, buildLocationIndex, parseSearchQuery, matchesAny } from '$lib/search';

	let mapContainer: HTMLDivElement;
	let map: maplibregl.Map;

	// Panel states
	let isAboutOpen = true;
	let isStatsOpen = false;
	let isSearchOpen = false;
	let isDarkTheme = false;

	// Stats
	let stats: PublicStats | null = null;

	// Search state
	let searchQuery = '';
	let searchTimeout: ReturnType<typeof setTimeout>;
	let fullGeoJSON: GeoJSON.FeatureCollection | null = null;

	let fqdnIndex: FQDNEntry[] = [];
	let locationIndex: LocationEntry[] = [];
	let displayedEntries: SearchEntry[] = [];

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

	function handleSearchInput(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		searchQuery = value;

		// Debounce the filtering
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			applyFilter(value);
		}, 150);
	}

	function excludeEntry(entry: SearchEntry, event: Event) {
		event.stopPropagation();
		const term = isFQDNEntry(entry) ? entry.fqdn : entry.rootDomain;
		// Add exclusion term to search query
		const newQuery = searchQuery ? `${searchQuery} -${term}` : `-${term}`;
		searchQuery = newQuery;
		applyFilter(newQuery);
	}

	function applyFilter(query: string) {
		const { includeTerms, excludeTerms } = parseSearchQuery(query);
		const hasIncludes = includeTerms.length > 0;
		const hasExcludes = excludeTerms.length > 0;

		if (!hasIncludes && !hasExcludes) {
			// No query: show recent locations (deduplicated by feature), all points on map
			displayedEntries = locationIndex.slice(0, 50);
			if (fullGeoJSON && map.getSource('loc-records')) {
				(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(fullGeoJSON);
			}
		} else if (hasIncludes) {
			// Has include terms: filter to matching FQDNs, then apply exclusions
			let matchingEntries = fqdnIndex.filter((entry) => matchesAny(entry.fqdn, includeTerms));

			if (hasExcludes) {
				matchingEntries = matchingEntries.filter((entry) => !matchesAny(entry.fqdn, excludeTerms));
			}

			displayedEntries = matchingEntries.slice(0, 50);

			// Filter map to only show features with matching FQDNs
			if (fullGeoJSON && map.getSource('loc-records')) {
				const matchingFeatures = new Set(matchingEntries.map((e) => e.feature));
				const filteredGeoJSON: GeoJSON.FeatureCollection = {
					type: 'FeatureCollection',
					features: fullGeoJSON.features.filter((f) => matchingFeatures.has(f))
				};
				(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(filteredGeoJSON);
			}
		} else {
			// Only exclude terms: show all locations except excluded
			const filteredLocations = locationIndex.filter(
				(entry) => !matchesAny(entry.rootDomain, excludeTerms)
			);
			displayedEntries = filteredLocations.slice(0, 50);

			// Filter map: exclude features where ANY fqdn matches exclusion
			if (fullGeoJSON && map.getSource('loc-records')) {
				const filteredGeoJSON: GeoJSON.FeatureCollection = {
					type: 'FeatureCollection',
					features: fullGeoJSON.features.filter((f) => {
						const fqdns = f.properties?.fqdns;
						const fqdnList: string[] = typeof fqdns === 'string' ? JSON.parse(fqdns) : fqdns || [];
						// Exclude if ANY fqdn matches any exclude term
						return !fqdnList.some((fqdn) => matchesAny(fqdn, excludeTerms));
					})
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
			const rootDomains =
				typeof props?.root_domains === 'string'
					? JSON.parse(props.root_domains)
					: props?.root_domains || [];

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
			existingPopups.forEach((p) => p.remove());

			new maplibregl.Popup().setLngLat(coords).setDOMContent(container).addTo(map);
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

	onMount(async () => {
		// Set initial theme and overlay state
		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		isDarkTheme = mediaQuery.matches;

		// Collapse about by default on small screens (< 768px)
		isAboutOpen = window.innerWidth >= 768;
		// Stats always starts collapsed

		// Load stats in parallel
		loadStats();

		// Fetch GeoJSON first so we can initialize map at the right bounds
		let initialBounds: maplibregl.LngLatBoundsLike | undefined;
		try {
			const response = await fetch('/api/public/records.geojson');
			if (response.ok) {
				const geojson: GeoJSON.FeatureCollection = await response.json();
				fullGeoJSON = geojson;
				fqdnIndex = buildFQDNIndex(geojson);
				locationIndex = buildLocationIndex(geojson);
				displayedEntries = locationIndex.slice(0, 50);

				// Calculate bounds for initial view
				if (geojson.features.length > 0) {
					const bounds = new maplibregl.LngLatBounds();
					for (const feature of geojson.features) {
						const coords = (feature.geometry as GeoJSON.Point).coordinates as [number, number];
						bounds.extend(coords);
					}
					initialBounds = bounds;
				}
			}
		} catch (e) {
			console.error('Failed to pre-fetch GeoJSON:', e);
		}

		// Create map with initial bounds (no zoom jump)
		const mapOptions: maplibregl.MapOptions = {
			container: mapContainer,
			style: getStyleUrl()
		};
		if (initialBounds) {
			mapOptions.bounds = initialBounds;
			mapOptions.fitBoundsOptions = { padding: 50, maxZoom: 10 };
		} else {
			mapOptions.center = [0, 30];
			mapOptions.zoom = 2;
		}

		map = new maplibregl.Map(mapOptions);
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
			// If we pre-fetched successfully, just add the layer
			if (fullGeoJSON) {
				addGeoJSONLayer(fullGeoJSON, true);
			} else {
				// Fallback: fetch now if pre-fetch failed
				await loadLOCRecords(true);
			}
		});

		return () => {
			mediaQuery.removeEventListener('change', handleThemeChange);
			map?.remove();
		};
	});

	function addGeoJSONLayer(geojson: GeoJSON.FeatureCollection, isInitialLoad = false) {
		// Add or update the source
		if (map.getSource('loc-records')) {
			(map.getSource('loc-records') as maplibregl.GeoJSONSource).setData(geojson);
			return;
		}

		map.addSource('loc-records', {
			type: 'geojson',
			data: geojson
		});

		// On initial load, start with points invisible to avoid chunky appearance
		// They'll fade in once everything is rendered
		map.addLayer({
			id: 'points',
			type: 'circle',
			source: 'loc-records',
			paint: {
				'circle-radius': 8,
				'circle-color': '#e74c3c',
				'circle-stroke-width': 2,
				'circle-stroke-color': '#fff',
				'circle-opacity': isInitialLoad ? 0 : 1,
				'circle-stroke-opacity': isInitialLoad ? 0 : 1
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
			const rootDomains =
				typeof props?.root_domains === 'string'
					? JSON.parse(props.root_domains)
					: props?.root_domains || [];

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

		// On initial load, wait for map to be fully rendered before showing points
		if (isInitialLoad) {
			map.once('idle', () => {
				// Fade in the points smoothly
				map.setPaintProperty('points', 'circle-opacity', 1);
				map.setPaintProperty('points', 'circle-stroke-opacity', 1);
			});
		}
	}

	async function loadLOCRecords(isInitialLoad = false) {
		try {
			const response = await fetch('/api/public/records.geojson');
			if (!response.ok) throw new Error('Failed to fetch records');

			const geojson: GeoJSON.FeatureCollection = await response.json();

			// Store for filtering and build search indices
			fullGeoJSON = geojson;
			fqdnIndex = buildFQDNIndex(geojson);
			locationIndex = buildLocationIndex(geojson);
			displayedEntries = locationIndex.slice(0, 50);

			addGeoJSONLayer(geojson, isInitialLoad);

			// Fit to data bounds on theme change (not initial load - that's handled by map constructor)
			if (!isInitialLoad && geojson.features.length > 0) {
				const bounds = new maplibregl.LngLatBounds();
				for (const feature of geojson.features) {
					const coords = (feature.geometry as GeoJSON.Point).coordinates as [number, number];
					bounds.extend(coords);
				}
				map.fitBounds(bounds, {
					padding: 50,
					maxZoom: 10,
					duration: 0
				});
			}
		} catch (error) {
			console.error('Error loading LOC records:', error);
		}
	}
</script>

<div id="map" bind:this={mapContainer}></div>

<div class="panels-container" class:dark={isDarkTheme}>
	<CollapsiblePanel title="About LOC.place" isOpen={isAboutOpen} onToggle={toggleAbout}>
		<p>
			As one of the old, core pieces internet infrastructure, the DNS system has many obscure and
			forgotten corners. One of those is the <a href="https://en.wikipedia.org/wiki/LOC_record"
				>LOC record</a
			>, which ties a domain name to a set of geographical coordinates. There are only a few
			thousand of these records in the entirety of DNS, making it feasible to map all of them.
		</p>
		<p>
			A massive thank you to tb0hdan for <a href="https://github.com/tb0hdan/domains/"
				>this list of domains</a
			>, and to my colleagues for taking it as a personal challenge to run as many scanners as they
			could.
		</p>
		<p>
			You can find the source code on <a href="https://github.com/locplace/locplace">github</a>. If
			you have any questions, remarks, or you just want to say hi, don't hesitate to
			<a href="mailto:contact@loc.place">email me</a>.
		</p>
	</CollapsiblePanel>

	{#if stats}
		<CollapsiblePanel
			title="Statistics"
			isOpen={isStatsOpen}
			onToggle={toggleStats}
			contentClass="stats-content"
		>
			<div class="stat-row">
				<span class="stat-label">LOC records</span>
				<span class="stat-value">{stats.total_loc_records.toLocaleString()}</span>
			</div>
			{#if stats.unique_locations !== undefined}
				<div class="stat-row">
					<span class="stat-label">Unique locations</span>
					<span class="stat-value">{stats.unique_locations.toLocaleString()}</span>
				</div>
			{/if}
			<div class="stat-row">
				<span class="stat-label">Unique apex domains</span>
				<span class="stat-value">{stats.unique_root_domains_with_loc.toLocaleString()}</span>
			</div>
		</CollapsiblePanel>
	{/if}

	<CollapsiblePanel
		title="Search"
		isOpen={isSearchOpen}
		onToggle={toggleSearch}
		contentClass="search-content"
	>
		<div class="search-input-wrapper">
			<input
				type="text"
				placeholder="Search FQDNs..."
				value={searchQuery}
				oninput={handleSearchInput}
				class="search-input"
			/>
			{#if searchQuery}
				<button
					class="clear-search"
					onclick={() => {
						searchQuery = '';
						applyFilter('');
					}}>×</button
				>
			{/if}
		</div>
		<div class="fqdn-list">
			{#if displayedEntries.length === 0}
				<div class="no-results">No matching FQDNs</div>
			{:else}
				{#each displayedEntries as entry}
					<div class="fqdn-item">
						<button class="fqdn-name" onclick={() => selectEntry(entry)}>
							{isFQDNEntry(entry) ? entry.fqdn : entry.rootDomain}
						</button>
						<button
							class="exclude-btn"
							onclick={(e) => excludeEntry(entry, e)}
							title="Hide from map">−</button
						>
					</div>
				{/each}
			{/if}
		</div>
	</CollapsiblePanel>
</div>

<style>
	/* Prevent scroll on map page - it should fill viewport */
	:global(html),
	:global(body) {
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

	@media (max-width: 400px) {
		.panels-container {
			max-width: calc(100vw - 20px);
		}
	}

	/* Stats panel specific styles */
	:global(.stats-content) {
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

	/* Search panel styles */
	:global(.search-content) {
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
		display: flex;
		align-items: center;
		width: 100%;
		border-radius: 3px;
	}

	.fqdn-item:hover {
		background: rgba(0, 0, 0, 0.08);
	}

	:global(.panels-container.dark) .fqdn-item:hover {
		background: rgba(255, 255, 255, 0.1);
	}

	.fqdn-name {
		flex: 1;
		padding: 6px 8px;
		text-align: left;
		background: none;
		border: none;
		cursor: pointer;
		font-size: 12px;
		font-family: monospace;
		color: inherit;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.exclude-btn {
		padding: 4px 8px;
		background: none;
		border: none;
		cursor: pointer;
		color: #999;
		font-size: 14px;
		font-weight: bold;
		opacity: 0;
		transition: opacity 0.15s;
	}

	.fqdn-item:hover .exclude-btn {
		opacity: 1;
	}

	.exclude-btn:hover {
		color: #e74c3c;
	}

	:global(.panels-container.dark) .exclude-btn {
		color: #666;
	}

	:global(.panels-container.dark) .exclude-btn:hover {
		color: #e74c3c;
	}

	.no-results {
		padding: 8px;
		text-align: center;
		opacity: 0.6;
		font-size: 12px;
	}
</style>
