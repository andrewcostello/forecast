// D2 Diagram Integration for Forecast Dashboard

// Diagram types
const DIAGRAM_TYPES = {
    BURNDOWN: 'burndown',
    FLOW: 'flow',
    TIMELINE: 'timeline'
};

// Load and render a diagram for a project
async function loadDiagram(projectKey, type = DIAGRAM_TYPES.BURNDOWN) {
    const container = document.getElementById('project-diagram');
    if (!container) return;

    container.innerHTML = `
        <h3>Diagram</h3>
        <div class="diagram-controls">
            <select id="diagram-type" onchange="switchDiagram('${projectKey}', this.value)">
                <option value="burndown" ${type === 'burndown' ? 'selected' : ''}>Burndown</option>
                <option value="flow" ${type === 'flow' ? 'selected' : ''}>Flow</option>
                <option value="timeline" ${type === 'timeline' ? 'selected' : ''}>Timeline</option>
            </select>
        </div>
        <div id="diagram-content" class="loading">Loading diagram...</div>
    `;

    try {
        const response = await fetch(`/api/projects/${projectKey}/diagram?type=${type}`);
        const diagramContent = document.getElementById('diagram-content');

        if (!response.ok) {
            const error = await response.json();
            diagramContent.innerHTML = `
                <p style="color: var(--text-secondary); padding: 2rem; text-align: center;">
                    ${error.message || 'Diagram generation not available'}
                </p>
            `;
            return;
        }

        // Check content type for SVG
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('image/svg+xml')) {
            const svg = await response.text();
            diagramContent.innerHTML = svg;
        } else {
            const data = await response.json();
            if (data.svg) {
                diagramContent.innerHTML = data.svg;
            } else if (data.d2) {
                // If we get D2 source, show it as code for now
                diagramContent.innerHTML = `<pre style="color: var(--text-secondary);">${escapeHtml(data.d2)}</pre>`;
            }
        }
    } catch (error) {
        const diagramContent = document.getElementById('diagram-content');
        diagramContent.innerHTML = `
            <p class="error">Failed to load diagram: ${escapeHtml(error.message)}</p>
        `;
    }
}

// Switch diagram type
function switchDiagram(projectKey, type) {
    loadDiagram(projectKey, type);
}

// Generate simple SVG burndown chart (client-side fallback)
function generateBurndownSVG(data) {
    if (!data || !data.points || data.points.length === 0) {
        return '<p>No data available for burndown chart</p>';
    }

    const width = 600;
    const height = 300;
    const padding = 40;

    const points = data.points;
    const maxY = Math.max(...points.map(p => p.remaining));

    const xScale = (width - padding * 2) / (points.length - 1 || 1);
    const yScale = (height - padding * 2) / (maxY || 1);

    const pathData = points.map((p, i) => {
        const x = padding + i * xScale;
        const y = height - padding - p.remaining * yScale;
        return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
    }).join(' ');

    return `
        <svg viewBox="0 0 ${width} ${height}" xmlns="http://www.w3.org/2000/svg">
            <style>
                .axis { stroke: #4a5568; stroke-width: 1; }
                .grid { stroke: #2d3748; stroke-width: 0.5; }
                .line { fill: none; stroke: #e94560; stroke-width: 2; }
                .point { fill: #e94560; }
                .label { fill: #a0a0a0; font-size: 12px; font-family: sans-serif; }
            </style>

            <!-- Grid lines -->
            ${Array.from({length: 5}, (_, i) => {
                const y = padding + i * (height - padding * 2) / 4;
                return `<line class="grid" x1="${padding}" y1="${y}" x2="${width - padding}" y2="${y}"/>`;
            }).join('')}

            <!-- Axes -->
            <line class="axis" x1="${padding}" y1="${padding}" x2="${padding}" y2="${height - padding}"/>
            <line class="axis" x1="${padding}" y1="${height - padding}" x2="${width - padding}" y2="${height - padding}"/>

            <!-- Data line -->
            <path class="line" d="${pathData}"/>

            <!-- Data points -->
            ${points.map((p, i) => {
                const x = padding + i * xScale;
                const y = height - padding - p.remaining * yScale;
                return `<circle class="point" cx="${x}" cy="${y}" r="4"/>`;
            }).join('')}

            <!-- Labels -->
            <text class="label" x="${width / 2}" y="${height - 10}" text-anchor="middle">Days</text>
            <text class="label" x="15" y="${height / 2}" transform="rotate(-90, 15, ${height / 2})" text-anchor="middle">Items</text>
        </svg>
    `;
}

// Escape HTML for safe rendering
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
