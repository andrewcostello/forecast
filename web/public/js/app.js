// Forecast Dashboard Application

const API_BASE = '/api';

// State
let currentProject = null;

// DOM Elements
const projectsSection = document.getElementById('projects-section');
const detailSection = document.getElementById('detail-section');
const projectsList = document.getElementById('projects-list');
const projectTitle = document.getElementById('project-title');
const projectStats = document.getElementById('project-stats');
const projectForecast = document.getElementById('project-forecast');
const projectDiagram = document.getElementById('project-diagram');
const backBtn = document.getElementById('back-btn');
const healthStatus = document.getElementById('health-status');

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    checkHealth();
    loadProjects();

    backBtn.addEventListener('click', showProjectsList);
});

// Health check
async function checkHealth() {
    try {
        const response = await fetch(`${API_BASE}/health`);
        const data = await response.json();

        healthStatus.classList.toggle('healthy', data.status === 'ok');
        healthStatus.classList.toggle('error', data.status !== 'ok');
        healthStatus.title = `Status: ${data.status}`;
    } catch (error) {
        healthStatus.classList.add('error');
        healthStatus.title = 'Server unreachable';
    }
}

// Load projects list
async function loadProjects() {
    try {
        const response = await fetch(`${API_BASE}/projects`);
        const data = await response.json();

        if (!data.projects || data.projects.length === 0) {
            projectsList.innerHTML = `
                <div class="error">
                    No projects configured. Add projects to your .forecast/config.yaml
                </div>
            `;
            return;
        }

        projectsList.innerHTML = data.projects.map(project => `
            <div class="project-card" onclick="showProject('${project.key}')">
                <h3>${escapeHtml(project.name)}</h3>
                <div class="epic">${escapeHtml(project.epic)}</div>
                <div class="progress-bar">
                    <div class="progress-fill ${project.progress < 50 ? 'warning' : ''}"
                         style="width: ${project.progress}%"></div>
                </div>
                <div class="stats-row">
                    <span>${project.done}/${project.total} done</span>
                    <span>${project.progress.toFixed(1)}%</span>
                </div>
            </div>
        `).join('');
    } catch (error) {
        projectsList.innerHTML = `
            <div class="error">
                Failed to load projects: ${escapeHtml(error.message)}
            </div>
        `;
    }
}

// Show project detail
async function showProject(key) {
    currentProject = key;
    projectsSection.classList.add('hidden');
    detailSection.classList.remove('hidden');

    projectStats.innerHTML = '<div class="loading">Loading project details...</div>';
    projectForecast.innerHTML = '';
    projectDiagram.innerHTML = '';

    try {
        // Load project detail
        const detailResponse = await fetch(`${API_BASE}/projects/${key}`);
        const detail = await detailResponse.json();

        if (detail.error) {
            throw new Error(detail.message);
        }

        projectTitle.textContent = detail.name;

        projectStats.innerHTML = `
            <div class="stat-card">
                <div class="value">${detail.total}</div>
                <div class="label">Total Items</div>
            </div>
            <div class="stat-card">
                <div class="value">${detail.done}</div>
                <div class="label">Completed</div>
            </div>
            <div class="stat-card">
                <div class="value">${detail.inProgress}</div>
                <div class="label">In Progress</div>
            </div>
            <div class="stat-card">
                <div class="value">${detail.remaining}</div>
                <div class="label">Remaining</div>
            </div>
            <div class="stat-card">
                <div class="value">${detail.progress.toFixed(1)}%</div>
                <div class="label">Progress</div>
            </div>
            ${detail.avgCycleHours ? `
                <div class="stat-card">
                    <div class="value">${detail.avgCycleHours.toFixed(1)}h</div>
                    <div class="label">Avg Cycle</div>
                </div>
            ` : ''}
        `;

        // Load forecast
        loadForecast(key);

    } catch (error) {
        projectStats.innerHTML = `
            <div class="error">
                Failed to load project: ${escapeHtml(error.message)}
            </div>
        `;
    }
}

// Load forecast data
async function loadForecast(key) {
    try {
        const response = await fetch(`${API_BASE}/projects/${key}/forecast`);
        const forecast = await response.json();

        if (forecast.error) {
            projectForecast.innerHTML = `
                <h3>Forecast</h3>
                <p class="error">${escapeHtml(forecast.message)}</p>
            `;
            return;
        }

        const percentiles = forecast.percentiles || {};

        projectForecast.innerHTML = `
            <h3>Monte Carlo Forecast</h3>
            <table class="forecast-table">
                <thead>
                    <tr>
                        <th>Confidence</th>
                        <th>Days</th>
                        <th>Target Date</th>
                    </tr>
                </thead>
                <tbody>
                    ${Object.entries(percentiles).map(([pct, data]) => `
                        <tr>
                            <td>${pct}%</td>
                            <td>${Math.round(data.days)}</td>
                            <td>${formatDate(data.targetDate)}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
            <p style="margin-top: 1rem; color: var(--text-secondary); font-size: 0.85rem;">
                Based on ${forecast.completed} completed items with avg cycle time of ${forecast.avgCycleHours.toFixed(1)}h.
                Throughput: ${forecast.throughputPerDay.toFixed(2)} items/day.
            </p>
        `;
    } catch (error) {
        projectForecast.innerHTML = `
            <h3>Forecast</h3>
            <p class="error">Failed to load forecast: ${escapeHtml(error.message)}</p>
        `;
    }
}

// Show projects list
function showProjectsList() {
    currentProject = null;
    detailSection.classList.add('hidden');
    projectsSection.classList.remove('hidden');
    loadProjects();
}

// Utility functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric'
    });
}

// Refresh health check every 30 seconds
setInterval(checkHealth, 30000);
