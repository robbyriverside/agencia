# helpline_langgraph.py
"""LangGraph implementation of the Helpline agent (Agencia helpline_spec.yaml)

* Compatible with **LangChain 0.1+** (`langchain-openai`).
* Loads `.env` for `OPENAI_API_KEY`.
* Single-node LangGraph loop until the LLM stops calling tools.
"""
from __future__ import annotations

import sys
from typing import List, Optional, Dict

from dotenv import load_dotenv
import yaml
from pathlib import Path
from pydantic import BaseModel, Field

from langchain_openai import ChatOpenAI
from langchain.prompts import ChatPromptTemplate, MessagesPlaceholder
from langchain.tools import StructuredTool
from langchain.agents import create_openai_tools_agent, AgentExecutor
from langchain.schema import AIMessage, BaseMessage
from langgraph.graph import StateGraph

# ---------------------------------------------------------------------------
# 0.  ENVIRONMENT -------------------------------------------------------------
# ---------------------------------------------------------------------------

load_dotenv()  # pulls OPENAI_API_KEY, etc.

# ---------------------------------------------------------------------------
# 1.  STATE -------------------------------------------------------------------
# ---------------------------------------------------------------------------

class HelplineState(dict):
    """Conversation state with helper properties for info & history."""

    @property
    def information(self) -> List[str]:
        return self.setdefault("information", [])

    @property
    def chat_history(self) -> List[BaseMessage]:
        return self.setdefault("chat_history", [])

    def add_info(self, text: str) -> None:
        if text and text not in self.information:
            self.information.append(text)

# ---------------------------------------------------------------------------
# 2.  TOOLS -------------------------------------------------------------------
# ---------------------------------------------------------------------------

class NursingArgs(BaseModel):
    date: Optional[str] = Field(None)
    time: Optional[str] = Field(None)
    nurse: Optional[str] = Field(None)
    address: Optional[str] = Field(None)

def nursing_tool(**kwargs) -> str:
    args = NursingArgs(**kwargs)
    if all([args.nurse, args.date, args.time, args.address]):
        return (
            f"I have scheduled your appointment with {args.nurse} on {args.date} at {args.time}. "
            f"The appointment will be held at {args.address}."
        )
    missing = [k for k, v in args.model_dump().items() if v is None]
    return "I’m missing " + ", ".join(missing) + ". Let me get back to you shortly with options."

class AppointmentArgs(BaseModel):
    date: Optional[str] = Field(None)
    time: Optional[str] = Field(None)
    doctor: Optional[str] = Field(None)
    location: Optional[str] = Field(None)

def appointments_tool(**kwargs) -> str:
    args = AppointmentArgs(**kwargs)
    if all([args.doctor, args.date, args.time, args.location]):
        return (
            f"I have scheduled your appointment with {args.doctor} on {args.date} at {args.time}. "
            f"The appointment will be held at {args.location}."
        )
    missing = [k for k, v in args.model_dump().items() if v is None]
    return "I’m missing " + ", ".join(missing) + ". Let me get back to you shortly with options."

class CognitiveArgs(BaseModel):
    name: Optional[str] = Field(None)

def cognitive_tool(**kwargs) -> str:
    name = CognitiveArgs(**kwargs).name or "friend"
    return (
        f"Hi {name}, I’d like to ask a few quick questions to see how you’re doing today. "
        "How have you been feeling mentally and emotionally?"
    )

def make_tool(name: str, description: str, schema: BaseModel, fn):
    return StructuredTool.from_function(
        fn,
        name=name,
        description=description,
        args_schema=schema,
        return_direct=True,
    )

TOOLS = [
    make_tool("nursing", "Schedule in-home nursing care", NursingArgs, nursing_tool),
    make_tool("appointments", "Schedule a doctor’s appointment", AppointmentArgs, appointments_tool),
    make_tool("cognitive", "Cognitive health check-in", CognitiveArgs, cognitive_tool),
]

# ---------------------------------------------------------------------------
# 3.  PROMPT BUILDER ----------------------------------------------------------
# ---------------------------------------------------------------------------

MAIN_MENU = (
    "You are a personal assistant for seniors.\n"
    "They call for help with: 1) doctor appointments, 2) in-home nursing care, 3) a cognitive check-in.\n"
    "If the caller starts talking, follow along and gently gather details.\n\n"
    "{info_block}\n"
    "User request:\n{user_input}"
)

def build_prompt(state: Dict) -> str:
    """Construct the prompt string safely even if keys are missing."""
    info_block = (
        "Here’s what the user already told us:
" + "
".join(f"- {x}" for x in state.get("facts", []))
        if state.get("facts") else ""
    )
    user_text = state.get("user_input", "")
    return MAIN_MENU.format(info_block=info_block, user_input=user_text))

# ---------------------------------------------------------------------------
# 4.  AGENT & EXECUTOR --------------------------------------------------------
# ---------------------------------------------------------------------------

llm = ChatOpenAI(model="gpt-4o-mini", temperature=0)

prompt = ChatPromptTemplate.from_messages([
    ("system", "You are Helpline, an empathetic senior-care assistant."),
    ("human", "{input}"),
    MessagesPlaceholder(variable_name="agent_scratchpad"),  # required placeholder
])

AGENT = create_openai_tools_agent(llm=llm, tools=TOOLS, prompt=prompt)
EXECUTOR = AgentExecutor(agent=AGENT, tools=TOOLS, verbose=False)

# ---------------------------------------------------------------------------
# 5.  LANGGRAPH (single-node loop) -------------------------------------------
# ---------------------------------------------------------------------------

def helpline_node(state: dict) -> dict:
    """LangGraph node: ensure we work with HelplineState helpers even if runner
    hands us a plain dict."""
    hs = HelplineState(state)  # wrap to get .information, .chat_history helpers

    result = EXECUTOR.invoke({
        "input": build_prompt(hs),
        "chat_history": hs.chat_history,
    })

    hs.chat_history.append(AIMessage(content=result["output"]))
    hs.add_info(hs["user_input"])

    return dict(hs)  # LangGraph prefers plain dicts

def done(state: HelplineState) -> bool:
    if not state.chat_history:
        return False
    last = state.chat_history[-1]
    return not getattr(last, "additional_kwargs", {}).get("function_call")

graph = StateGraph(HelplineState)
graph.add_node("helpline", helpline_node)
graph.set_entry_point("helpline")
SINGLE_STEP_GRAPH = graph.compile()

# ---------------------------------------------------------------------------
# 6.  CLI ---------------------------------------------------------------------
# ---------------------------------------------------------------------------

def run_dialog(dialog: List[dict]):
    """Run a single dialog (list of {input, output} turns) and print comparison."""
    state: Dict = {"chat": [], "facts": []}

    for turn in dialog:
        state["user_input"] = turn["input"]
        # LangGraph step(s) until assistant stops calling tools
        while True:
            state = SINGLE_STEP_GRAPH.invoke(state)
            if done(state):
                break
        assistant_resp = state["chat"][-1].content
        print(f"User:      {turn['input']}")
        print(f"Expected:  {turn['output']}")
        print(f"Assistant: {assistant_resp}
")

    print("=" * 40)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python helpline_langgraph.py <yaml file | user message>")
        sys.exit(1)

    first_arg = sys.argv[1]
    path = Path(first_arg)

    if path.suffix in {".yml", ".yaml"} and path.exists():
        # Load YAML with multiple scripts
        with open(path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
        scripts = data.get("scripts", [])
        for idx, script in enumerate(scripts, 1):
            dialog = script.get("dialog", [])
            print(f"Running dialog #{idx}\n{'-'*40}")
            run_dialog(dialog)
    else:
        # Treat arg as a one-off user message
        state = {"user_input": first_arg}
        while True:
            state = SINGLE_STEP_GRAPH.invoke(state)
            if done(state):
                break
        print("Assistant response:
-------------------")
        print(state["chat"][-1].content)
