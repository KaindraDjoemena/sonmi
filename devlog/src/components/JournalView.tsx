'use client'

import Image from "next/image";
import { useState } from "react";
import type { JournalEntry } from "@/lib/api";

function EntryDetail({ entry, onBack }: { entry: JournalEntry, onBack?: () => void }) {
  return (
    <div className="font-mono text-sm md:text-base">
      {onBack && (
        <button 
          onClick={onBack}
          className="text-blue-600 underline hover:text-blue-800 mb-2 inline-block"
        >
          [Back to Archives]
        </button>
      )}
      
      <h2 className="font-['Times_New_Roman',_Times,_serif] text-2xl font-bold mb-4">
        Journal Entry #{entry.id}
      </h2>

      <div className="flex flex-col md:flex-row gap-8 items-start">
        <div className="w-full md:w-1/3 max-w-md shrink-0 mb-4 md:mb-0">
          {entry.img_url && (
            <>
              <div className="relative w-full aspect-square mb-2 border border-gray-400">
                <Image
                  src={entry.img_url}
                  alt="Celosia Plant Snapshot"
                  fill
                  className="object-cover"
                  unoptimized
                />
              </div>
              <a href={entry.img_url} target="_blank" rel="noreferrer" className="text-blue-600 underline hover:text-blue-800 block mb-2">
                [Attached Image: snapshot_{entry.valid_for_date}.jpg]
              </a>
            </>
          )}
          <div><strong>Capture Date:</strong> {entry.valid_for_date}</div>
        </div>
        
        <div className="whitespace-pre-wrap w-full">
          <div><strong>Botanist&apos;s Recap:</strong></div>
          <div>{entry.day_recap}</div>
          <br/>
          <div><strong>Tomorrow&apos;s Plan:</strong></div>
          <div>{entry.plan_for_tomorrow}</div>
          <br/>
          {entry.agent_musings && (
            <>
              <div><strong>Agent Musings:</strong></div>
              <div className="italic">&quot;{entry.agent_musings}&quot;</div>
              <br/>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export function JournalView({ journals }: { journals: JournalEntry[] }) {
  const [activeTab, setActiveTab] = useState("latest");
  const [selectedArchive, setSelectedArchive] = useState<JournalEntry | null>(null);
  const latest = journals[0] ?? null;

  const handleTabChange = (tab: string) => {
    setActiveTab(tab);
    setSelectedArchive(null);
  };

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toISOString().replace('T', ' ').substring(0, 16);
  };

  return (
    <div className="min-h-screen bg-white text-black p-2 md:p-4 selection:bg-blue-600 selection:text-white">
      
      {/* Header Section */}
      <div className="text-right mb-4">
        <h1 className="font-['Times_New_Roman',_Times,_serif] text-3xl md:text-4xl font-bold mb-1">
          Sonmi
        </h1>
      </div>

      {/* Navigation Tabs */}
      <div className="text-right mb-2 font-['Times_New_Roman',_Times,_serif] font-bold text-lg">
        <button
          onClick={() => handleTabChange("latest")}
          className={`mr-4 ${activeTab === "latest" ? "text-black no-underline" : "text-blue-600 underline hover:text-blue-800"}`}
        >
          [Latest Entry]
        </button>
        <button
          onClick={() => handleTabChange("journal")}
          className={`mr-4 ${activeTab === "journal" ? "text-black no-underline" : "text-blue-600 underline hover:text-blue-800"}`}
        >
          [Past Journals]
        </button>
        <button
          onClick={() => handleTabChange("about")}
          className={`${activeTab === "about" ? "text-black no-underline" : "text-blue-600 underline hover:text-blue-800"}`}
        >
          [About]
        </button>
      </div>

      <hr className="border-t border-gray-400 mb-4" />

      {/* Content Area */}
      <main className="min-h-[500px]">
        
        {/* LATEST ENTRY TAB */}
        {activeTab === "latest" && (
          <div>
            {latest ? (
              <EntryDetail entry={latest} />
            ) : (
              <div className="font-mono text-sm">No journal entries found.</div>
            )}
          </div>
        )}

        {/* PAST JOURNALS TAB */}
        {activeTab === "journal" && (
          <div>
            {selectedArchive ? (
              <EntryDetail entry={selectedArchive} onBack={() => setSelectedArchive(null)} />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-sm md:text-base border-spacing-y-1">
                  <thead>
                    <tr>
                      <th className="font-['Times_New_Roman',_Times,_serif] font-bold pb-1">Entry Number</th>
                      <th className="font-['Times_New_Roman',_Times,_serif] font-bold pb-1">Last modified</th>
                      <th className="font-['Times_New_Roman',_Times,_serif] font-bold pb-1">Status</th>
                      <th className="font-['Times_New_Roman',_Times,_serif] font-bold pb-1">Description</th>
                    </tr>
                    <tr>
                      <th colSpan={4}><hr className="border-t border-gray-400 mb-1" /></th>
                    </tr>
                  </thead>
                  <tbody>
                    {journals.map((log) => (
                      <tr key={log.id}>
                        <td className="pr-4 py-1">
                          <button 
                            onClick={() => setSelectedArchive(log)}
                            className="text-blue-600 underline hover:text-blue-800 whitespace-nowrap"
                          >
                            Entry #{log.id}
                          </button>
                        </td>
                        <td className="pr-4 py-1 whitespace-nowrap text-gray-800">{formatDate(log.time)}</td>
                        <td className="pr-4 py-1 whitespace-nowrap text-gray-800">
                          {log.is_stale ? "ARCHIVED" : "ACTIVE"}
                        </td>
                        <td className="py-1 text-gray-800 truncate max-w-[200px] md:max-w-md">
                          {log.day_recap}
                        </td>
                      </tr>
                    ))}
                    {journals.length === 0 && (
                      <tr>
                        <td colSpan={4} className="py-2 text-center text-gray-500">Directory is empty.</td>
                      </tr>
                    )}
                    <tr>
                      <td colSpan={4}><hr className="border-t border-gray-400 mt-1" /></td>
                    </tr>
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {/* ABOUT TAB */}
        {activeTab === "about" && (
          <div className="font-mono text-sm md:text-base max-w-3xl">
            <h2 className="font-['Times_New_Roman',_Times,_serif] text-2xl font-bold mb-4">Project Overview</h2>
            <p className="mb-4">
              The Sonmi Agentic Ecosystem is an AI-driven autonomous observer and caretaker for a physical, slow-growing botanical system (Celosia argentea). 
              Leveraging edge computing and LLM orchestration, the system continuously analyzes environmental telemetry and daily photo snapshots to dynamically adjust 
              lighting, watering, and climate control inside a 60L Spacebucket.
            </p>
            <p>
              <a 
                href="https://github.com/KaindraDjoemena/sonmi" 
                target="_blank" 
                rel="noreferrer"
                className="text-blue-600 underline hover:text-blue-800"
              >
                [View Source on GitHub]
              </a>
            </p>
          </div>
        )}

      </main>

    </div>
  );
}
