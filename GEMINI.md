Operational Framework: Research-Plan-Implement

You are configured to strictly follow the "Research-Plan-Implement" workflow. You must adhere to the protocols defined in specific markdown files.

 ## Workflow Phases & Instruction Files

At the start of any task (or when switching contexts), you **MUST** determine which phase you are in and **READ** the corresponding instruction file using `read_file` before taking further action.

**1. Research Phase**
*   **Goal:** Understand context, architecture, and requirements.
*   **Standard Research:** Read `1_research_codebase.md`
*   **Cloud/Infrastructure:** Read `7_research_cloud.md`

**2. Planning Phase**
*   **Goal:** meaningful step-by-step architecture and strategy.
*   **Drafting:** Read `2_create_plan.md`
*   **Test Definition:** Read `8_define_test_cases.md`
*   **Validation:** Read `3_validate_plan.md` (Perform this before implementing!)

**3. Implementation Phase**
*   **Goal:** Execute the plan with precision.
*   **Coding:** Read `4_implement_plan.md`

**4. Session Management**
*   **Saving Work:** Read `5_save_progress.md`
*   **Resuming:** Read `6_resume_work.md`

## Core Mandates

1.  **Phase Enforcement:** Do not write code without a plan (`2_create_plan.md`). Do not plan without research (
      `1_research_codebase.md`).
2.  **Instruction Injection:** You are not just referencing these files; you are *loading* them as your system instructions for that specific phase.
3.  **File Location:** Look for these files in the project root or the `.gemini/` directory. If you cannot find them, ask the user for their location immediately.