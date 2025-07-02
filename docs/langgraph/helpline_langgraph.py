# helpline_langgraph_min.py
from __future__ import annotations
import sys
from typing import Dict, List, Optional
import textwrap

from dotenv import load_dotenv
from pydantic import BaseModel, Field
from langchain_openai import ChatOpenAI
from langchain.prompts import ChatPromptTemplate, MessagesPlaceholder
from langchain.tools import StructuredTool
from langchain.agents import AgentExecutor, create_openai_tools_agent
from langchain.schema import AIMessage
from langgraph.graph import StateGraph

# ── ENV ────────────────────────────────────────────────────────────────────
load_dotenv()            # expects OPENAI_API_KEY in .env
llm = ChatOpenAI(model="gpt-4o-mini", temperature=0)

# ── TOOLS ──────────────────────────────────────────────────────────────────
class AppointmentArgs(BaseModel):
    date: Optional[str] = Field(None)
    time: Optional[str] = Field(None)
    doctor: Optional[str] = Field(None)
    location: Optional[str] = Field(None)

def appointment_tool(**kw) -> str:
    a = AppointmentArgs(**kw)
    if all(a.model_dump().values()):
        return textwrap.dedent(f"""\
            I’ve scheduled Dr {a.doctor} on {a.date} at {a.time} at {a.location}.
            """).strip()
    missing = [k for k, v in a.model_dump().items() if v is None]
    return f"I’m missing {', '.join(missing)}. Let’s sort that out."

class NursingArgs(BaseModel):
    date: Optional[str] = Field(None)
    time: Optional[str] = Field(None)
    nurse: Optional[str] = Field(None)
    location: Optional[str] = Field(None)

def nursing_tool(**kw) -> str:
    n = NursingArgs(**kw)
    if all(n.model_dump().values()):
        return textwrap.dedent(f"""\
            I’ve scheduled nurse {n.nurse} on {n.date} at {n.time} at {n.location}.
            """).strip()
    missing = [k for k, v in n.model_dump().items() if v is None]
    return f"I’m missing {', '.join(missing)}. Let’s sort that out."

class CognitiveArgs(BaseModel):
    date: Optional[str] = Field(None)
    time: Optional[str] = Field(None)
    location: Optional[str] = Field(None)

def cognitive_tool(**kw) -> str:
    c = CognitiveArgs(**kw)
    if all(c.model_dump().values()):
        return textwrap.dedent(f"""\
            I’ve scheduled a cognitive check-in on {c.date} at {c.time} at {c.location}.
            """).strip()
    missing = [k for k, v in c.model_dump().items() if v is None]
    return f"I’m missing {', '.join(missing)}. Let’s sort that out."

def make_tool(name, desc, schema, fn):
    return StructuredTool.from_function(fn, name=name,
                                        description=desc,
                                        args_schema=schema,
                                        return_direct=True)

TOOLS = [
    make_tool("appointments", "Schedule doctor appointment", AppointmentArgs, appointment_tool),
    make_tool("nursing", "Schedule in-home nursing care", NursingArgs, nursing_tool),
    make_tool("cognitive", "Schedule cognitive check-in", CognitiveArgs, cognitive_tool),
]

MAINMENU_PROMPT = textwrap.dedent("""
You are a personal assistant for seniors. 
Seniors call you to get help with the following tasks.:
1. Schedule a doctor's appointment
2. Schedule in-home nursing care
3. Cognative check-in

If the caller begins talking about something, 
don’t redirect — follow along and gently guide 
the conversation to gather details as needed.
{information_block}
Pay attention to what kind of appointment the user is asking about.
If they seem confused and you can't figure out what they want,
schedule a cognitive check-in.

User request:
{user_input}
""")

def build_prompt(state: Dict) -> str:
    info_list = state.get("facts", [])
    print(f"*** Facts: {info_list}")
    if info_list:
        info_text = "Here’s what the user has already told us so far. Use this information to avoid repeating questions or asking irrelevant ones:\\n" + \
                    "\n".join(f"- {item}" for item in info_list) + \
                    "\nThis context includes known names, times, places, and services they've mentioned. Use it to figure out what kind of help they need."
    else:
        info_text = ""
    return MAINMENU_PROMPT.format(information_block=info_text, user_input=state.get("user_input",""))

prompt = ChatPromptTemplate.from_messages([
    ("system", "You are Helpline, an empathetic senior‑care assistant."),
    ("human", "{input}"),
    MessagesPlaceholder(variable_name="agent_scratchpad"),
])

AGENT  = create_openai_tools_agent(llm=llm, tools=TOOLS, prompt=prompt)
XCTOR = AgentExecutor(agent=AGENT, tools=TOOLS, verbose=False)

# ── LANGGRAPH NODE ────────────────────────────────────────────────────────
def node(state: Dict) -> Dict:
    """Single LangGraph node: ask LLM, maybe call a tool, update history."""
    # Ensure this user utterance is already stored in facts BEFORE we build the prompt
    facts = state.setdefault("facts", [])
    if state.get("user_input") and state["user_input"] not in facts:
        facts.append(state["user_input"])
    print("--------------------------")
    print(build_prompt(state))
    print("--------------------------")
    out = XCTOR.invoke({"input": build_prompt(state),
                        "chat_history": state.setdefault("chat", [])})
    state["chat"].append(AIMessage(content=out["output"]))
    return state  # pure dict

def done(state: Dict) -> bool:
    last = state.get("chat", [])[-1]
    fc = getattr(last, "additional_kwargs", {}).get("function_call")
    return fc is None

# compile graph
g = StateGraph(dict)
g.add_node("step", node)
g.set_entry_point("step")
GRAPH = g.compile()

# ── CLI LOOP ──────────────────────────────────────────────────────────────
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python helpline_langgraph_min.py \"<user message>\"")
        sys.exit(1)

    state: Dict = {"user_input": sys.argv[1]}
    while True:
        state = GRAPH.invoke(state)
        if done(state):
            break

    print("Assistant response:\n-------------------")
    print(state["chat"][-1].content)
    for m in state["chat"]:
        fc = getattr(m, "additional_kwargs", {}).get("function_call")
        if fc:
            print(f"[Tool call] {fc}")