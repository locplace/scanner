<script lang="ts">
	import { onMount, mount } from 'svelte';
	import maplibregl, { type GeoJSONSource } from 'maplibre-gl';
	import MapPopup from '$lib/components/MapPopup.svelte';

	let mapContainer: HTMLDivElement;
	let map: maplibregl.Map;

	// Protomaps API key - get yours at https://protomaps.com/api
	const PROTOMAPS_API_KEY = import.meta.env.VITE_PROTOMAPS_API_KEY || 'YOUR_API_KEY_HERE';

	onMount(() => {
		const styleUrl = `https://api.protomaps.com/styles/v5/light/en.json?key=${PROTOMAPS_API_KEY}`;

		map = new maplibregl.Map({
			container: mapContainer,
			style: styleUrl,
			center: [0, 30],
			zoom: 2
		});

		map.addControl(new maplibregl.NavigationControl(), 'top-right');

		map.on('load', async () => {
			await loadLOCRecords();
		});

		return () => {
			map?.remove();
		};
	});

	async function loadLOCRecords() {
		try {
			const response = await fetch('/api/public/records.geojson');
			if (!response.ok) throw new Error('Failed to fetch records');

			const geojson = await response.json();

			// Add source with clustering for geographically nearby points
			map.addSource('loc-records', {
				type: 'geojson',
				data: geojson,
				cluster: true,
				clusterMaxZoom: 17,
				clusterRadius: 40,
				generateId: true, // Stable IDs for proper feature tracking
				tolerance: 0 // Don't simplify points
			});

			// Cluster circles (for geographically nearby aggregated locations)
			map.addLayer({
				id: 'clusters',
				type: 'circle',
				source: 'loc-records',
				filter: ['has', 'point_count'],
				paint: {
					'circle-color': '#c0392b',
					'circle-radius': ['step', ['get', 'point_count'], 18, 5, 24, 10, 30],
					'circle-stroke-width': 2,
					'circle-stroke-color': '#fff'
				}
			});

			// Cluster count labels
			map.addLayer({
				id: 'cluster-count',
				type: 'symbol',
				source: 'loc-records',
				filter: ['has', 'point_count'],
				layout: {
					'text-field': ['get', 'point_count_abbreviated'],
					'text-size': 12
				},
				paint: {
					'text-color': '#fff'
				}
			});

			// Unclustered points (individual aggregated locations)
			map.addLayer({
				id: 'unclustered-point',
				type: 'circle',
				source: 'loc-records',
				filter: ['!', ['has', 'point_count']],
				paint: {
					'circle-radius': 8,
					'circle-color': '#e74c3c',
					'circle-stroke-width': 2,
					'circle-stroke-color': '#fff',
					'circle-opacity': 0.85
				}
			});

			// Click handler for supercluster clusters
			map.on('click', 'clusters', async (e) => {
				if (!e.features?.length) return;

				const feature = e.features[0];
				const clusterId = feature.properties?.cluster_id;
				const coords = (feature.geometry as GeoJSON.Point).coordinates as [number, number];
				const source = map.getSource('loc-records') as GeoJSONSource;

				// Get the zoom level needed to expand this cluster
				const expansionZoom = await source.getClusterExpansionZoom(clusterId);

				// If we can zoom in more, do that
				if (expansionZoom <= 17 && map.getZoom() < expansionZoom) {
					map.easeTo({ center: coords, zoom: expansionZoom });
					return;
				}

				// Otherwise show popup with all FQDNs from all locations in the cluster
				const leaves = await source.getClusterLeaves(clusterId, 100, 0);
				if (leaves.length === 0) return;

				// Aggregate all FQDNs from all locations
				const allFqdns: string[] = [];
				const allRootDomains = new Set<string>();
				let rawRecord = '';
				let altitudeM = 0;

				for (const leaf of leaves) {
					const props = leaf.properties;
					// Parse fqdns - it comes as JSON string from MapLibre
					const fqdns = typeof props?.fqdns === 'string' ? JSON.parse(props.fqdns) : props?.fqdns || [];
					const rootDomains = typeof props?.root_domains === 'string' ? JSON.parse(props.root_domains) : props?.root_domains || [];

					allFqdns.push(...fqdns);
					rootDomains.forEach((d: string) => allRootDomains.add(d));

					if (!rawRecord && props?.raw_record) {
						rawRecord = props.raw_record;
						altitudeM = props.altitude_m || 0;
					}
				}

				const container = document.createElement('div');
				mount(MapPopup, {
					target: container,
					props: {
						fqdns: allFqdns,
						rootDomains: Array.from(allRootDomains),
						latitude: coords[1],
						longitude: coords[0],
						altitudeM,
						rawRecord
					}
				});

				new maplibregl.Popup().setLngLat(coords).setDOMContent(container).addTo(map);
			});

			// Click handler for unclustered points (individual aggregated locations)
			map.on('click', 'unclustered-point', (e) => {
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

			// Change cursor on hover for both layers
			for (const layer of ['clusters', 'unclustered-point']) {
				map.on('mouseenter', layer, () => {
					map.getCanvas().style.cursor = 'pointer';
				});
				map.on('mouseleave', layer, () => {
					map.getCanvas().style.cursor = '';
				});
			}

			// Fit to data bounds if we have records
			if (geojson.features.length > 0) {
				const bounds = new maplibregl.LngLatBounds();
				for (const feature of geojson.features) {
					bounds.extend(feature.geometry.coordinates as [number, number]);
				}
				map.fitBounds(bounds, { padding: 50, maxZoom: 10 });
			}

			// Force repaint after map becomes idle to ensure all points render
			map.once('idle', () => {
				map.triggerRepaint();
			});
		} catch (error) {
			console.error('Error loading LOC records:', error);
		}
	}
</script>

<div id="map" bind:this={mapContainer}></div>
