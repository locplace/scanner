<script lang="ts">
	import { onMount, mount } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import MapPopup from '$lib/components/MapPopup.svelte';

	let mapContainer: HTMLDivElement;
	let map: maplibregl.Map;

	// Protomaps API key - get yours at https://protomaps.com/api
	const PROTOMAPS_API_KEY = import.meta.env.VITE_PROTOMAPS_API_KEY || 'YOUR_API_KEY_HERE';

	onMount(() => {
		// Use Protomaps hosted Style JSON - the simplest approach
		// Available themes: light, dark, white, black, grayscale
		// Available languages: en, de, fr, es, etc.
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

			map.addSource('loc-records', {
				type: 'geojson',
				data: geojson
			});

			// Add circle layer for points
			map.addLayer({
				id: 'loc-points',
				type: 'circle',
				source: 'loc-records',
				paint: {
					'circle-radius': 8,
					'circle-color': '#e74c3c',
					'circle-stroke-width': 2,
					'circle-stroke-color': '#fff',
					'circle-opacity': 0.85
				}
			});

			// Add click handler for popups
			map.on('click', 'loc-points', (e) => {
				if (!e.features?.length) return;

				const feature = e.features[0];
				const props = feature.properties;
				const coords = (feature.geometry as GeoJSON.Point).coordinates;

				// Use Svelte component for safe HTML rendering (prevents XSS)
				const container = document.createElement('div');
				mount(MapPopup, {
					target: container,
					props: {
						fqdn: props.fqdn,
						rootDomain: props.root_domain,
						latitude: coords[1],
						longitude: coords[0],
						altitudeM: props.altitude_m,
						rawRecord: props.raw_record
					}
				});

				new maplibregl.Popup()
					.setLngLat(coords as [number, number])
					.setDOMContent(container)
					.addTo(map);
			});

			// Change cursor on hover
			map.on('mouseenter', 'loc-points', () => {
				map.getCanvas().style.cursor = 'pointer';
			});

			map.on('mouseleave', 'loc-points', () => {
				map.getCanvas().style.cursor = '';
			});

			// Fit to data bounds if we have records
			if (geojson.features.length > 0) {
				const bounds = new maplibregl.LngLatBounds();
				for (const feature of geojson.features) {
					bounds.extend(feature.geometry.coordinates as [number, number]);
				}
				map.fitBounds(bounds, { padding: 50, maxZoom: 10 });
			}
		} catch (error) {
			console.error('Error loading LOC records:', error);
		}
	}
</script>

<div id="map" bind:this={mapContainer}></div>
