const state = {
  apiKey: localStorage.getItem("searchapi.key") || "",
  health: null,
  metrics: {},
  assets: [],
  runs: [],
  channels: [],
  alarms: [],
  events: [],
  samples: [],
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];

document.addEventListener("DOMContentLoaded", () => {
  $("#apiKey").value = state.apiKey;
  bindTabs();
  bindDialogs();
  bindActions();
  loadAll();
});

function bindTabs() {
  $$(".tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      $$(".tab").forEach((item) => item.classList.remove("active"));
      $$(".panel").forEach((panel) => panel.classList.remove("active"));
      tab.classList.add("active");
      $(`#${tab.dataset.panel}`).classList.add("active");
    });
  });
}

function bindDialogs() {
  $$("[data-open]").forEach((button) => {
    button.addEventListener("click", () => $(`#${button.dataset.open}`).showModal());
  });

  $("#assetForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const payload = formJSON(event.target);
    await request("/api/v1/assets", { method: "POST", body: payload });
    $("#assetDialog").close();
    event.target.reset();
    toast("Asset created");
    loadAll();
  });

  $("#runForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const payload = { ...formJSON(event.target), status: "active" };
    await request("/api/v1/test-runs", { method: "POST", body: payload });
    $("#runDialog").close();
    event.target.reset();
    toast("Run created");
    loadAll();
  });

  $("#channelForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const payload = formJSON(event.target);
    if (payload.alarm_max) {
      payload.alarm_max = Number(payload.alarm_max);
    } else {
      delete payload.alarm_max;
    }
    payload.data_type = "float64";
    await request("/api/v1/channels", { method: "POST", body: payload });
    $("#channelDialog").close();
    event.target.reset();
    toast("Channel created");
    loadAll();
  });
}

function bindActions() {
  $("#authForm").addEventListener("submit", (event) => {
    event.preventDefault();
    state.apiKey = $("#apiKey").value.trim();
    localStorage.setItem("searchapi.key", state.apiKey);
    toast("API key stored locally");
    loadAll();
  });

  $("#refreshBtn").addEventListener("click", loadAll);

  $("#seedBtn").addEventListener("click", async () => {
    const summary = await request("/api/v1/onboarding/seed", { method: "POST" });
    toast(`Seeded ${summary.asset_id}`);
    loadAll();
  });

  $("#loadExampleBtn").addEventListener("click", () => {
    $("#streamBody").value = [
      '{"run_id":"run_seed_acceptance_001","channel_id":"chan_accel_x","gateway_id":"gateway-sea-01","timestamp":"2026-07-07T19:00:01Z","numeric_value":2.4,"quality":"good","replay_key":"ui-accel-0001"}',
      '{"run_id":"run_seed_acceptance_001","channel_id":"chan_accel_x","gateway_id":"gateway-sea-01","timestamp":"2026-07-07T19:00:02Z","numeric_value":8.1,"quality":"good","replay_key":"ui-accel-0002","metadata":{"phase":"threshold-check"}}',
      '{"run_id":"run_seed_acceptance_001","channel_id":"chan_case_temp","gateway_id":"gateway-sea-01","timestamp":"2026-07-07T19:00:03Z","numeric_value":42.7,"quality":"good","replay_key":"ui-temp-0001"}',
    ].join("\n");
  });

  $("#sendStreamBtn").addEventListener("click", async () => {
    const text = $("#streamBody").value.trim();
    if (!text) {
      toast("Paste JSON or NDJSON first", true);
      return;
    }
    const result = await request("/api/v1/ingest/stream", {
      method: "POST",
      rawBody: text,
      contentType: "application/x-ndjson",
    });
    $("#ingestResult").textContent = JSON.stringify(result, null, 2);
    $("#ingestState").textContent = "accepted";
    toast(`Accepted ${result.accepted_samples} samples`);
    loadAll();
  });

  $("#searchForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const query = $("#searchQuery").value.trim();
    if (!query) {
      return;
    }
    const hits = await request(`/api/v1/search?q=${encodeURIComponent(query)}`);
    renderSearch(hits);
  });
}

async function loadAll() {
  try {
    state.health = await request("/healthz", { public: true });
    render();

    const protectedLoads = await Promise.allSettled([
      request("/metrics"),
      request("/api/v1/assets"),
      request("/api/v1/test-runs"),
      request("/api/v1/channels"),
      request("/api/v1/alarms?status=open"),
      request("/api/v1/events?limit=30"),
    ]);
    const rejected = protectedLoads.find((result) => result.status === "rejected");
    if (rejected) {
      throw rejected.reason;
    }
    const [metrics, assets, runs, channels, alarms, events] = protectedLoads.map((result) => result.value);
    Object.assign(state, { metrics, assets, runs, channels, alarms, events });
    extractSamples(events);
    render();
  } catch (error) {
    toast(error.message, true);
    render();
  }
}

async function request(path, options = {}) {
  const headers = { Accept: "application/json" };
  if (!options.public && state.apiKey) {
    headers.Authorization = `Bearer ${state.apiKey}`;
  }
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.contentType) {
    headers["Content-Type"] = options.contentType;
  }
  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    body: options.rawBody ?? (options.body === undefined ? undefined : JSON.stringify(options.body)),
  });
  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(data.error || `${response.status} ${response.statusText}`);
  }
  return data;
}

function render() {
  $("#siteLine").textContent = state.health
    ? `${state.health.site_id} / ${state.health.environment}`
    : "Backend unavailable";
  $("#databaseState").textContent = state.health?.database || "unknown";
  $("#elasticState").textContent = state.health?.elasticsearch ? "enabled" : "fallback";
  $("#sampleCount").textContent = state.metrics.accepted_samples || 0;
  $("#alarmCount").textContent = state.alarms.length;
  $("#alarmLabel").textContent = `${state.alarms.length} active`;
  $("#stripLabel").textContent = `${state.samples.length} plotted samples`;

  renderList("#assetList", state.assets, assetItem);
  renderList("#runList", state.runs, runItem);
  renderList("#channelList", state.channels, channelItem);
  renderList("#alarmList", state.alarms, alarmItem, "No open alarms");
  renderList("#eventList", state.events, eventItem, "No events yet");
  drawTelemetry();
}

function renderList(selector, rows, renderer, empty = "No records yet") {
  const host = $(selector);
  host.innerHTML = "";
  if (!rows || rows.length === 0) {
    host.innerHTML = `<div class="item"><small>${empty}</small></div>`;
    return;
  }
  for (const row of rows) {
    const node = document.createElement("article");
    node.className = "item";
    node.innerHTML = renderer(row);
    host.appendChild(node);
  }
}

function assetItem(asset) {
  return `<strong>${escapeHTML(asset.name)}</strong>
    <small>${escapeHTML(asset.id)} / ${escapeHTML(asset.kind)} / ${escapeHTML(asset.location || "unplaced")}</small>
    <span class="badge">${escapeHTML((asset.tags || []).join(", ") || "asset")}</span>`;
}

function runItem(run) {
  return `<strong>${escapeHTML(run.run_number)}</strong>
    <small>${escapeHTML(run.id)} / ${escapeHTML(run.asset_id)} / ${escapeHTML(run.operator || "operator")}</small>
    <span class="badge">${escapeHTML(run.status)}</span>`;
}

function channelItem(channel) {
  const max = channel.alarm_max === undefined ? "" : ` max ${channel.alarm_max}`;
  return `<strong>${escapeHTML(channel.name)}</strong>
    <small>${escapeHTML(channel.id)} / ${escapeHTML(channel.asset_id)} / ${escapeHTML(channel.unit || "unitless")}${max}</small>
    <span class="badge">${escapeHTML(channel.data_type)}</span>`;
}

function alarmItem(alarm) {
  return `<strong>${escapeHTML(alarm.message)}</strong>
    <small>${escapeHTML(alarm.id)} / ${escapeHTML(alarm.channel_id)} / ${new Date(alarm.raised_at).toLocaleString()}</small>
    <span class="badge danger">${escapeHTML(alarm.severity)} / ${escapeHTML(alarm.status)}</span>`;
}

function eventItem(event) {
  return `<strong>${escapeHTML(event.action)} ${escapeHTML(event.subject)}</strong>
    <small>${escapeHTML(event.id)} / ${escapeHTML(event.event_type)} / ${new Date(event.occurred_at).toLocaleString()}</small>
    <span class="badge">${escapeHTML(event.actor)}</span>`;
}

function renderSearch(hits) {
  renderList("#searchResults", hits, (hit) => {
    const source = typeof hit.source === "string" ? hit.source : JSON.stringify(hit.source);
    return `<strong>${escapeHTML(hit.kind)} / ${escapeHTML(hit.id)}</strong>
      <small>${escapeHTML(source).slice(0, 360)}</small>
      <span class="badge">score ${hit.score || 0}</span>`;
  }, "No hits");
}

function extractSamples(events) {
  const samples = [];
  for (const event of events || []) {
    if (event.event_type !== "telemetry_batch") {
      continue;
    }
    const payload = parsePayload(event.payload);
    for (const sample of payload.samples || []) {
      if (typeof sample.numeric_value === "number") {
        samples.push(sample);
      }
    }
  }
  state.samples = samples.slice(-80);
}

function parsePayload(payload) {
  if (!payload) {
    return {};
  }
  if (typeof payload === "string") {
    try {
      return JSON.parse(payload);
    } catch {
      return {};
    }
  }
  return payload;
}

function drawTelemetry() {
  const canvas = $("#telemetryCanvas");
  const ctx = canvas.getContext("2d");
  const width = canvas.width;
  const height = canvas.height;
  ctx.clearRect(0, 0, width, height);
  ctx.fillStyle = "rgba(0, 12, 18, 0.8)";
  ctx.fillRect(0, 0, width, height);
  ctx.strokeStyle = "rgba(185, 230, 255, 0.12)";
  ctx.lineWidth = 1;
  for (let x = 0; x < width; x += 60) {
    ctx.beginPath();
    ctx.moveTo(x, 0);
    ctx.lineTo(x, height);
    ctx.stroke();
  }
  for (let y = 0; y < height; y += 42) {
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(width, y);
    ctx.stroke();
  }
  const samples = state.samples;
  if (samples.length < 2) {
    ctx.fillStyle = "rgba(198, 243, 255, 0.72)";
    ctx.fillText("Seed or ingest telemetry to plot samples", 28, 42);
    return;
  }
  const values = samples.map((sample) => sample.numeric_value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const spread = max - min || 1;
  ctx.strokeStyle = "#5de7ff";
  ctx.lineWidth = 3;
  ctx.beginPath();
  samples.forEach((sample, index) => {
    const x = (index / (samples.length - 1)) * (width - 40) + 20;
    const y = height - 24 - ((sample.numeric_value - min) / spread) * (height - 48);
    if (index === 0) {
      ctx.moveTo(x, y);
    } else {
      ctx.lineTo(x, y);
    }
  });
  ctx.stroke();
  ctx.fillStyle = "#64f4bc";
  for (const [index, sample] of samples.entries()) {
    const x = (index / (samples.length - 1)) * (width - 40) + 20;
    const y = height - 24 - ((sample.numeric_value - min) / spread) * (height - 48);
    ctx.beginPath();
    ctx.arc(x, y, 4, 0, Math.PI * 2);
    ctx.fill();
  }
}

function formJSON(form) {
  const data = new FormData(form);
  const out = {};
  for (const [key, value] of data.entries()) {
    const clean = String(value).trim();
    if (clean !== "") {
      out[key] = clean;
    }
  }
  return out;
}

function toast(message, error = false) {
  const node = $("#toast");
  node.textContent = message;
  node.style.borderColor = error ? "rgba(255, 107, 138, 0.48)" : "rgba(100, 244, 188, 0.32)";
  node.classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => node.classList.remove("show"), 2800);
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}
