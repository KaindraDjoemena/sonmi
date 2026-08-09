"use client";

import Image from "next/image";
import { useState } from "react";
import { latestJournal } from "@/lib/mock-data";

// Add some dummy historical journals for the table
const pastJournals = [
  { id: 452, date: "2026-08-08", summary: "Excellent turgor pressure, growth steady.", status: "Nominal", time: "23:58" },
  { id: 451, date: "2026-08-07", summary: "Adjusted lighting to counter stem elongation.", status: "Correction", time: "23:59" },
  { id: 450, date: "2026-08-06", summary: "Soil moisture dropped to 52%, triggered pump.", status: "Action Taken", time: "23:58" },
  { id: 449, date: "2026-08-05", summary: "Initial germination signs confirmed. System initialized.", status: "Nominal", time: "23:57" },
];

export default function Home() {
  const [activeTab, setActiveTab] = useState("latest");
  const safeDefaults = JSON.parse(latestJournal.safe_defaults_json);

  return (
    <div className="min-h-screen bg-[#e0e0e0] text-black font-mono p-4 md:p-8 selection:bg-black selection:text-white">
      <div className="max-w-4xl mx-auto border-4 border-black bg-white shadow-[8px_8px_0px_0px_rgba(0,0,0,1)]">
        
        {/* Header Section */}
        <header className="border-b-4 border-black p-6 bg-black text-white flex flex-col md:flex-row justify-between items-start md:items-end gap-4">
          <div>
            <h1 className="text-4xl md:text-6xl font-serif tracking-tight uppercase mb-2">Sonmi Devlog</h1>
            <p className="text-sm md:text-base border-t border-white/30 pt-2 uppercase">
              <span className="font-bold">S.O.N.M.I:</span> Sonmi Oversees Nature&apos;s Microclimate Interface
            </p>
          </div>
          <div className="text-right">
            <div className="text-xs uppercase mb-1">System Status</div>
            <div className="bg-white text-black px-2 py-1 font-bold text-sm inline-block">NOMINAL</div>
          </div>
        </header>

        {/* Navigation Tabs */}
        <nav className="flex border-b-4 border-black bg-[#f0f0f0] overflow-x-auto">
          {["latest", "journal", "about"].map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`flex-1 py-3 px-6 text-sm font-bold uppercase tracking-wider border-r-4 border-black last:border-r-0 hover:bg-[#d0d0d0] transition-colors ${
                activeTab === tab ? "bg-white underline decoration-2 underline-offset-4" : ""
              }`}
            >
              {tab === "latest" ? "Latest Entry" : tab === "journal" ? "Past Journals" : "About"}
            </button>
          ))}
        </nav>

        {/* Content Area */}
        <main className="p-6 md:p-10">
          
          {/* LATEST ENTRY TAB */}
          {activeTab === "latest" && (
            <div className="flex flex-col gap-8 animate-in fade-in duration-300">
              <div className="flex flex-col md:flex-row gap-6 border-2 border-black p-4 bg-[#f9f9f9]">
                <div className="w-full md:w-1/2 border-2 border-black p-1 bg-white">
                  <div className="relative aspect-square w-full filter grayscale hover:grayscale-0 transition-all duration-500">
                    <Image
                      src={latestJournal.img_url}
                      alt="Celosia Plant Snapshot"
                      fill
                      className="object-cover"
                      priority
                    />
                  </div>
                  <div className="text-xs uppercase p-2 text-center border-t-2 border-black font-bold">
                    Capture: {latestJournal.date}
                  </div>
                </div>
                
                <div className="w-full md:w-1/2 flex flex-col gap-4">
                  <h2 className="font-serif text-2xl uppercase border-b-2 border-black pb-2">Botanist&apos;s Recap</h2>
                  <p className="text-sm leading-relaxed">{latestJournal.day_recap}</p>
                  
                  <h2 className="font-serif text-2xl uppercase border-b-2 border-black pb-2 mt-2">Tomorrow&apos;s Plan</h2>
                  <p className="text-sm leading-relaxed">{latestJournal.plan_for_tomorrow}</p>
                  
                  <h2 className="font-serif text-2xl uppercase border-b-2 border-black pb-2 mt-2">Safe Defaults</h2>
                  <div className="bg-black text-white p-4 text-xs">
                    <div>LIGHT: {safeDefaults.light_schedule?.on} - {safeDefaults.light_schedule?.off}</div>
                    <div>PUMP_DUR: {safeDefaults.pump_duration_ms}ms</div>
                    <div>WATER_TRIG: &le; {safeDefaults.watering_threshold_percent}%</div>
                    <div>FAN_TEMP_TRIG: &ge; {safeDefaults.exhaust_fan_temp_trigger_c}&deg;C</div>
                    <div>FAN_HUM_TRIG: &ge; {safeDefaults.exhaust_fan_humidity_trigger_percent}%</div>
                  </div>
                </div>
              </div>

              <div className="border-2 border-black p-6 bg-black text-white">
                <h2 className="font-serif text-2xl uppercase border-b border-white/30 pb-2 mb-4">Agent Musings</h2>
                <p className="text-sm leading-relaxed italic">&quot;{latestJournal.agent_musings}&quot;</p>
              </div>
            </div>
          )}

          {/* PAST JOURNALS TAB */}
          {activeTab === "journal" && (
            <div className="animate-in fade-in duration-300">
              <h2 className="font-serif text-3xl uppercase border-b-4 border-black pb-4 mb-6">Database Archives</h2>
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse border-2 border-black">
                  <thead>
                    <tr className="bg-black text-white text-sm uppercase">
                      <th className="border-2 border-black p-3">Entry #</th>
                      <th className="border-2 border-black p-3">Date</th>
                      <th className="border-2 border-black p-3">Time</th>
                      <th className="border-2 border-black p-3">Summary</th>
                      <th className="border-2 border-black p-3">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pastJournals.map((log) => (
                      <tr 
                        key={log.id} 
                        className="text-sm border-2 border-black hover:bg-[#f0f0f0] cursor-pointer transition-colors"
                        onClick={() => alert(`In a real app, this would load Journal Entry #${log.id}!`)}
                      >
                        <td className="border-2 border-black p-3 font-bold">#{log.id}</td>
                        <td className="border-2 border-black p-3">{log.date}</td>
                        <td className="border-2 border-black p-3">{log.time}</td>
                        <td className="border-2 border-black p-3">{log.summary}</td>
                        <td className="border-2 border-black p-3">
                          <span className={`px-2 py-1 uppercase text-xs font-bold ${log.status === "Nominal" ? "bg-black text-white" : "border-2 border-black"}`}>
                            {log.status}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* ABOUT TAB */}
          {activeTab === "about" && (
            <div className="animate-in fade-in duration-300 max-w-2xl">
              <h2 className="font-serif text-3xl uppercase border-b-4 border-black pb-4 mb-6">Project Overview</h2>
              <p className="text-base leading-relaxed mb-6 bg-[#f9f9f9] border-l-4 border-black p-4">
                The Sonmi Agentic Ecosystem is an AI-driven autonomous observer and caretaker for a physical, slow-growing botanical system (Celosia argentea). 
                Leveraging edge computing and LLM orchestration, the system continuously analyzes environmental telemetry and daily photo snapshots to dynamically adjust 
                lighting, watering, and climate control inside a 60L Spacebucket.
              </p>
              
              <a 
                href="https://github.com/KaindraDjoemena/sonmi" 
                target="_blank" 
                rel="noreferrer"
                className="inline-block border-4 border-black px-6 py-3 font-bold uppercase tracking-wider hover:bg-black hover:text-white transition-colors"
              >
                GitHub ↗
              </a>
            </div>
          )}

        </main>
      </div>
    </div>
  );
}
